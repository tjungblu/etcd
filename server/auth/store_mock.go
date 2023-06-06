// Copyright 2021 The etcd Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth

import (
	"encoding/base64"

	"go.etcd.io/etcd/api/v3/authpb"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"golang.org/x/crypto/bcrypt"
)

func encodePassword(s string) string {
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(s), bcrypt.MinCost)
	return base64.StdEncoding.EncodeToString(hashedPassword)
}

// TestEnableAuthAndCreateRoot should be used for testing only.
func TestEnableAuthAndCreateRoot(as AuthStore) error {
	_, err := as.UserAdd(&pb.AuthUserAddRequest{Name: "root", HashedPassword: encodePassword("root"), Options: &authpb.UserAddOptions{NoPassword: false}})
	if err != nil {
		return err
	}

	_, err = as.RoleAdd(&pb.AuthRoleAddRequest{Name: "root"})
	if err != nil {
		return err
	}

	_, err = as.UserGrantRole(&pb.AuthUserGrantRoleRequest{User: "root", Role: "root"})
	if err != nil {
		return err
	}

	return as.AuthEnable()
}

type backendMock struct {
	users    map[string]*authpb.User
	roles    map[string]*authpb.Role
	enabled  bool
	revision uint64
}

// NewBackendMock should be used for testing only.
func NewBackendMock() AuthBackend {
	return &backendMock{
		users: make(map[string]*authpb.User),
		roles: make(map[string]*authpb.Role),
	}
}

func (b *backendMock) CreateAuthBuckets() {
}

func (b *backendMock) ForceCommit() {
}

func (b *backendMock) ReadTx() AuthReadTx {
	return &txMock{be: b}
}

func (b *backendMock) BatchTx() AuthBatchTx {
	return &txMock{be: b}
}

func (b *backendMock) GetUser(s string) *authpb.User {
	return b.users[s]
}

func (b *backendMock) GetAllUsers() []*authpb.User {
	return b.BatchTx().UnsafeGetAllUsers()
}

func (b *backendMock) GetRole(s string) *authpb.Role {
	return b.roles[s]
}

func (b *backendMock) GetAllRoles() []*authpb.Role {
	return b.BatchTx().UnsafeGetAllRoles()
}

var _ AuthBackend = (*backendMock)(nil)

type txMock struct {
	be *backendMock
}

var _ AuthBatchTx = (*txMock)(nil)

func (t txMock) UnsafeReadAuthEnabled() bool {
	return t.be.enabled
}

func (t txMock) UnsafeReadAuthRevision() uint64 {
	return t.be.revision
}

func (t txMock) UnsafeGetUser(s string) *authpb.User {
	return t.be.users[s]
}

func (t txMock) UnsafeGetRole(s string) *authpb.Role {
	return t.be.roles[s]
}

func (t txMock) UnsafeGetAllUsers() []*authpb.User {
	var users []*authpb.User
	for _, u := range t.be.users {
		users = append(users, u)
	}
	return users
}

func (t txMock) UnsafeGetAllRoles() []*authpb.Role {
	var roles []*authpb.Role
	for _, r := range t.be.roles {
		roles = append(roles, r)
	}
	return roles
}

func (t txMock) Lock() {
}

func (t txMock) Unlock() {
}

func (t txMock) UnsafeSaveAuthEnabled(enabled bool) {
	t.be.enabled = enabled
}

func (t txMock) UnsafeSaveAuthRevision(rev uint64) {
	t.be.revision = rev
}

func (t txMock) UnsafePutUser(user *authpb.User) {
	t.be.users[string(user.Name)] = user
}

func (t txMock) UnsafeDeleteUser(s string) {
	delete(t.be.users, s)
}

func (t txMock) UnsafePutRole(role *authpb.Role) {
	t.be.roles[string(role.Name)] = role
}

func (t txMock) UnsafeDeleteRole(s string) {
	delete(t.be.roles, s)
}
