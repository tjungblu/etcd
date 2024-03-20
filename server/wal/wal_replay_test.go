package wal

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/client/pkg/v3/fileutil"
	"go.etcd.io/etcd/server/v3/etcdserver/api/snap"
	"go.etcd.io/etcd/server/v3/wal/walpb"
	"go.uber.org/zap"
	"io"
	"os"
	"path"
	"testing"
)

func TestSnapshotReads(t *testing.T) {
	lg := zap.NewExample()
	dirpath := "/home/tjungblu/Downloads/abb-copy-of-crashed-etcd/"

	s := snap.New(lg, dirpath)
	snap, err := s.Load()
	require.NoError(t, err)

	fmt.Printf("%s\n", snap.String())
}

func TestABBReplay(t *testing.T) {
	lg := zap.NewExample()
	dirpath := "/home/tjungblu/Downloads/abb-copy-of-crashed-etcd/"
	names, err := readWALNames(lg, dirpath)
	require.NoError(t, err)

	var reader []fileutil.FileReader
	for _, name := range names {
		fPath := path.Join(dirpath, name)
		f, err := os.Open(fPath)
		require.NoError(t, err)
		defer f.Close()

		reader = append(reader, fileutil.NewFileReader(f))
	}

	d := newDecoder(reader...)

	rec := &walpb.Record{}
	for {
		err := d.decode(rec)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
		}

		// fmt.Printf("%s\n", rec.String())

		// update crc of the decoder when necessary
		if rec.Type == crcType {
			crc := d.crc.Sum32()
			// current crc of decoder must match the crc of the record.
			// do no need to match 0 crc, since the decoder is a new one at this case.
			if crc != 0 && rec.Validate(crc) != nil {
				t.Fail()
			}
			d.updateCRC(rec.Crc)
		} else if rec.Type == snapshotType {
			snap := &walpb.Snapshot{}
			err := snap.Unmarshal(rec.Data)
			require.NoError(t, err)

			fmt.Printf("snapshot: %s\n", snap.String())
		} else if rec.Type == stateType {
			state := mustUnmarshalState(rec.Data)

			fmt.Printf("raft state: %s\n", state.String())
		} else {
			e := mustUnmarshalEntry(rec.Data)
			fmt.Printf("%s\n", e.String())
		}
	}

}
