package apply

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.etcd.io/etcd/api/v3/authpb"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/server/v3/auth"
	"go.etcd.io/etcd/server/v3/lease"
	"go.uber.org/zap/zaptest"
)

func TestCheckLeasePutsKeys(t *testing.T) {
	aa := authApplierV3{as: auth.NewAuthStore(zaptest.NewLogger(t), auth.NewBackendMock(), &auth.TokenNop{}, 10)}
	assert.Nil(t, aa.checkLeasePutsKeys(lease.NewLease(lease.LeaseID(1), 3600)), "auth is disabled, should allow puts")
	assert.Nil(t, auth.TestEnableAuthAndCreateRoot(aa.as), "error while enabling auth")
	aa.authInfo = auth.AuthInfo{Username: "root"}
	assert.Nil(t, aa.checkLeasePutsKeys(lease.NewLease(lease.LeaseID(1), 3600)), "auth is enabled, should allow puts for root")

	l := lease.NewLease(lease.LeaseID(1), 3600)
	l.SetLeaseItem(lease.LeaseItem{Key: "a"})
	aa.authInfo = auth.AuthInfo{Username: "bob", Revision: 0}
	assert.ErrorIs(t, aa.checkLeasePutsKeys(l), auth.ErrUserEmpty, "auth is enabled, should not allow bob, non existing at rev 0")
	aa.authInfo = auth.AuthInfo{Username: "bob", Revision: 1}
	assert.ErrorIs(t, aa.checkLeasePutsKeys(l), auth.ErrAuthOldRevision, "auth is enabled, old revision")

	aa.authInfo = auth.AuthInfo{Username: "bob", Revision: aa.as.Revision()}
	assert.ErrorIs(t, aa.checkLeasePutsKeys(l), auth.ErrPermissionDenied, "auth is enabled, bob does not have permissions, bob does not exist")
	_, err := aa.as.UserAdd(&pb.AuthUserAddRequest{Name: "bob", Options: &authpb.UserAddOptions{NoPassword: true}})
	assert.Nil(t, err, "bob should be added without error")
	aa.authInfo = auth.AuthInfo{Username: "bob", Revision: aa.as.Revision()}
	assert.ErrorIs(t, aa.checkLeasePutsKeys(l), auth.ErrPermissionDenied, "auth is enabled, bob exists yet does not have permissions")

	// allow bob to access "a"
	_, err = aa.as.RoleAdd(&pb.AuthRoleAddRequest{Name: "bobsrole"})
	assert.Nil(t, err, "bobsrole should be added without error")
	_, err = aa.as.RoleGrantPermission(&pb.AuthRoleGrantPermissionRequest{
		Name: "bobsrole",
		Perm: &authpb.Permission{
			PermType: authpb.READWRITE,
			Key:      []byte("a"),
			RangeEnd: nil,
		},
	})
	assert.Nil(t, err, "bobsrole should be granted permissions without error")
	_, err = aa.as.UserGrantRole(&pb.AuthUserGrantRoleRequest{
		User: "bob",
		Role: "bobsrole",
	})
	assert.Nil(t, err, "bob should be granted bobsrole without error")

	aa.authInfo = auth.AuthInfo{Username: "bob", Revision: aa.as.Revision()}
	assert.Nil(t, aa.checkLeasePutsKeys(l), "bob should be able to access key 'a'")

}
