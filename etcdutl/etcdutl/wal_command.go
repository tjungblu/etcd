package etcdutl

import (
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.etcd.io/etcd/client/pkg/v3/types"
	"go.etcd.io/etcd/server/v3/datadir"
	"go.etcd.io/etcd/server/v3/etcdserver"
	"go.etcd.io/etcd/server/v3/etcdserver/api"
	"go.etcd.io/etcd/server/v3/etcdserver/api/membership"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/etcd/server/v3/etcdserver/api/v2store"
	"go.etcd.io/etcd/server/v3/mvcc/backend"
	"go.etcd.io/etcd/server/v3/wal"
)

func NewWALCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wal <subcommand>",
		Short: "Manages etcd node write-ahead-logs",
	}
	cmd.AddCommand(NewWalMemberRemoveCommand())
	cmd.AddCommand(NewWalMemberListCommand())

	cmd.PersistentFlags().StringVar(&dataDir, "data-dir", "", "Path to the etcd data dir")
	cmd.MarkFlagRequired("data-dir")

	return cmd
}

func NewWalMemberListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "member-list",
		Short: "list all members known to the current snapshot storage",
		RunE:  walMemberList,
	}
}

func NewWalMemberRemoveCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "member-remove <member-id>",
		Short: "Removes a member from raft by appending a config change member removal to the write-ahead-log.",
		RunE:  walMemberRemove,
	}
}

func walMemberList(_ *cobra.Command, _ []string) error {
	cl, _, _, be, err := recoverMembershipCluster()
	if err != nil {
		return err
	}

	defer be.Close()

	members := cl.Members()

	p := NewPrinter(OutputFormat)
	p.MemberList(members)

	return nil
}

func walMemberRemove(_ *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("unexpected number of arguments, expected only one member id")
	}
	cl, ss, st, be, err := recoverMembershipCluster()
	if err != nil {
		return err
	}

	memberId, err := types.IDFromString(args[0])
	if err != nil {
		return err
	}

	var foundMember *membership.Member
	members := cl.Members()
	for _, member := range members {
		if member.ID == memberId {
			foundMember = member.Clone()
			break
		}
	}

	if foundMember == nil {
		return fmt.Errorf("could not find member with id: %s", args[0])
	}

	cl.RemoveMember(foundMember.ID, true)
	cl.PushMembershipToStorage()
	be.ForceCommit()
	err = be.Close()
	if err != nil {
		return err
	}

	// we need to adjust the hard state to reflect that removal too
	walDir := datadir.ToWalDir(dataDir)
	walSnaps, err := wal.ValidSnapshotEntries(GetLogger(), walDir)
	if err != nil {
		return err
	}
	snapshot, err := ss.LoadNewestAvailable(walSnaps)
	if err != nil {
		return err
	}

	var votersFiltered []uint64
	for _, voter := range snapshot.Metadata.ConfState.Voters {
		if types.ID(voter) != memberId {
			votersFiltered = append(votersFiltered, voter)
		}
	}

	snapshot.Metadata.ConfState.Voters = votersFiltered
	// the snapshot data also contains the member nodes, so we need to save it along
	data, err := st.Save()
	if err != nil {
		return err
	}
	snapshot.Data = data
	err = ss.SaveSnap(*snapshot)
	if err != nil {
		return err
	}

	return nil
}

func recoverMembershipCluster() (*membership.RaftCluster, *snap.Snapshotter, v2store.Store, backend.Backend, error) {
	lg := GetLogger()

	walDir := datadir.ToWalDir(dataDir)
	snapDir := datadir.ToSnapDir(dataDir)
	backendFile := datadir.ToBackendFileName(dataDir)

	if err := fileutil.IsDirWriteable(walDir); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("cannot write to WAL directory: %v", err)
	}

	ss := snap.New(lg, snapDir)
	walSnaps, err := wal.ValidSnapshotEntries(lg, walDir)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	snapshot, err := ss.LoadNewestAvailable(walSnaps)
	if err != nil && !errors.Is(err, snap.ErrNoSnapshot) {
		return nil, nil, nil, nil, err
	}

	st := v2store.New(etcdserver.StoreClusterPrefix, etcdserver.StoreKeysPrefix)
	if err = st.Recovery(snapshot.Data); err != nil {
		return nil, nil, nil, nil, err
	}

	be := backend.NewDefaultBackend(backendFile)
	cl := membership.NewCluster(lg)
	cl.SetStore(st)
	cl.SetBackend(be)
	cl.Recover(api.UpdateCapability)
	return cl, ss, st, be, nil
}
