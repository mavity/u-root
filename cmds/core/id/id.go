// Copyright 2017-2024 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// id displays the user id, group id, and groups of the calling process.
//
// Synopsis:
//
//	id [-gGnu]
//
// Description:
//
//	id displays the uid, gid and groups of the calling process
//
// Options:
//
//	-g, --group     print only the effective group ID
//	-G, --groups    print all group IDs
//	-n, --name      print a name instead of a number, for -ugG
//	-u, --user      print only the effective user ID
//	-r, --real      print real ID instead of effective ID
package id

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/user"
	"sort"
	"strconv"
	"strings"

	"github.com/u-root/u-root/pkg/core"
	"github.com/u-root/u-root/pkg/uroot/unixflag"
)

const (
	groupFile  = "/etc/group"
	passwdFile = "/etc/passwd"
)

type flags struct {
	group  bool
	groups bool
	name   bool
	user   bool
	real   bool
}

var (
	errOnlyOneChoice     = errors.New("id: cannot print \"only\" of more than one choice")
	errNotOnlyNames      = errors.New("id: cannot print only names in default format")
	errNotOnlyNamesOrIDs = errors.New("id: cannot print only names or real IDs in default format")
)

func correctFlags(flags ...bool) bool {
	n := 0
	for _, v := range flags {
		if v {
			n++
		}
	}
	return n <= 1
}

// User contains user information, as from /etc/passwd
type User struct {
	groups map[int]string
	name   string
	uid    int
	gid    int
}

// UID returns the integer UID for a user
func (u *User) UID() int {
	return u.uid
}

// GID returns the integer GID for a user
func (u *User) GID() int {
	return u.gid
}

// Name returns the name for a user
func (u *User) Name() string {
	return u.name
}

// Groups returns all the groups for a user in a map
func (u *User) Groups() map[int]string {
	return u.groups
}

// GIDName returns the group name for a user's GID
func (u *User) GIDName() string {
	if name, ok := u.Groups()[u.GID()]; ok {
		return name
	}
	return strconv.Itoa(u.GID())
}

func newUserFromOS(f *flags, username string) (*User, error) {
	var currUser *user.User
	var err error
	if len(username) == 0 {
		currUser, err = user.Current()
	} else {
		currUser, err = user.Lookup(username)
		if err != nil {
			currUser, err = user.LookupId(username)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("id: unknown user or uid: %s", username)
	}

	u := &User{groups: make(map[int]string)}
	u.name = currUser.Username
	if idx := strings.LastIndex(u.name, `\`); idx >= 0 {
		u.name = u.name[idx+1:]
	}

	uid, err := strconv.Atoi(currUser.Uid)
	if err != nil {
		uid = sidToID(currUser.Uid, 1000)
	}
	u.uid = uid

	gid, err := strconv.Atoi(currUser.Gid)
	if err != nil {
		gid = sidToID(currUser.Gid, 1000)
	}
	u.gid = gid

	u.groups[gid] = u.name
	if gids, err := currUser.GroupIds(); err == nil {
		for _, gStr := range gids {
			gNum, err := strconv.Atoi(gStr)
			if err != nil {
				gNum = sidToID(gStr, gid)
			}
			gName := strconv.Itoa(gNum)
			if grp, err := user.LookupGroupId(gStr); err == nil && grp.Name != "" {
				gName = grp.Name
				if idx := strings.LastIndex(gName, `\`); idx >= 0 {
					gName = gName[idx+1:]
				}
			}
			u.groups[gNum] = gName
		}
	}
	return u, nil
}

func sidToID(sid string, fallback int) int {
	parts := strings.Split(sid, "-")
	if len(parts) > 1 {
		if rid, err := strconv.Atoi(parts[len(parts)-1]); err == nil && rid > 0 {
			return rid
		}
	}
	return fallback
}

// NewUser is a factory method for the User type.
func NewUser(flags *flags, username string, users *Users, groups *Groups) (*User, error) {
	var groupsNumbers []int

	u := &User{groups: make(map[int]string)}
	if len(username) == 0 { // no username provided, get current
		pUID, pGID, pGids := getProcessIDs(flags.real)
		if pUID < 0 {
			return newUserFromOS(flags, username)
		}
		u.uid = pUID
		u.gid = pGID
		groupsNumbers = pGids
		if v, err := users.GetUser(u.uid); err == nil {
			u.name = v
		} else {
			u.name = strconv.Itoa(u.uid)
		}
	} else {
		if v, err := users.GetUID(username); err == nil { // user is username
			u.name = username
			u.uid = v
		} else {
			if uid, err := strconv.Atoi(username); err == nil { // user is valid int
				if v, err := users.GetUser(uid); err == nil { // user is valid uid
					u.name = v
					u.uid = uid
				}
			} else {
				return nil, fmt.Errorf("id: no such user or uid: %s", username)
			}
		}
		u.gid, _ = users.GetGID(u.uid)
		groupsNumbers = append([]int{u.gid}, groups.UserGetGIDs(u.name)...)
	}

	for _, groupNum := range groupsNumbers {
		if groupName, err := groups.GetGroup(groupNum); err == nil {
			u.groups[groupNum] = groupName
		} else {
			u.groups[groupNum] = strconv.Itoa(groupNum)
		}
	}

	return u, nil
}

// IDCommand runs the "id" with the current user's information.
func IDCommand(w io.Writer, flags *flags, u User) {
	if !flags.groups {
		if flags.user {
			if flags.name {
				fmt.Fprintln(w, u.Name())
				return
			}
			fmt.Fprintln(w, u.UID())
			return
		} else if flags.group {
			if flags.name {
				fmt.Fprintln(w, u.GIDName())
				return
			}
			fmt.Fprintln(w, u.GID())
			return
		}

		fmt.Fprintf(w, "uid=%d(%s) ", u.UID(), u.Name())
		fmt.Fprintf(w, "gid=%d(%s) ", u.GID(), u.GIDName())
	}

	if !flags.groups {
		fmt.Fprintf(w, "groups=")
	}

	// handle Go map ordering
	var gids []int
	for gid := range u.Groups() {
		gids = append(gids, gid)
	}
	sort.Ints(gids)

	var groupOutput []string
	for _, id := range gids {
		gid, name := id, u.Groups()[id]
		if !flags.groups {
			groupOutput = append(groupOutput, fmt.Sprintf("%d(%s)", gid, name))
		} else {
			if flags.name {
				groupOutput = append(groupOutput, fmt.Sprintf("%s ", name))
			} else {
				groupOutput = append(groupOutput, fmt.Sprintf("%d ", gid))
			}
		}
	}

	sep := ","
	if flags.groups {
		sep = ""
	}

	fmt.Fprintln(w, strings.Join(groupOutput, sep))
}

func run(w io.Writer, name string, f *flags, passwd, group string) error {
	if !correctFlags(f.groups, f.group, f.user) {
		return errOnlyOneChoice
	}
	if f.name && (!f.groups && !f.group && !f.user) {
		return errNotOnlyNames
	}
	if len(name) != 0 && f.real {
		return errNotOnlyNamesOrIDs
	}

	users, errUsers := NewUsers(passwd)
	groups, errGroups := NewGroups(group)

	if passwd != passwdFile || group != groupFile {
		if errUsers != nil {
			return fmt.Errorf("id: %w", errUsers)
		}
		if errGroups != nil {
			return fmt.Errorf("id: %w", errGroups)
		}
	}

	var userObj *User
	var err error
	if errUsers == nil && errGroups == nil {
		userObj, err = NewUser(f, name, users, groups)
	}
	if userObj == nil {
		userObj, err = newUserFromOS(f, name)
		if err != nil {
			return fmt.Errorf("id: %w", err)
		}
	}

	IDCommand(w, f, *userObj)
	return nil
}

// Main runs the id command reentrantly.
func Main(ctx context.Context, ioEnv core.IO, args ...string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(ioEnv.Stderr, "id: panic: %v\n", r)
			exitCode = 1
		}
	}()

	f := &flags{}
	fs := flag.NewFlagSet("id", flag.ContinueOnError)
	fs.SetOutput(ioEnv.Stderr)

	fs.BoolVar(&f.group, "g", false, "print only the effective group ID")
	fs.BoolVar(&f.group, "group", false, "print only the effective group ID")
	fs.BoolVar(&f.groups, "G", false, "print all group IDs")
	fs.BoolVar(&f.groups, "groups", false, "print all group IDs")
	fs.BoolVar(&f.name, "n", false, "print a name instead of a number, for -ugG")
	fs.BoolVar(&f.name, "name", false, "print a name instead of a number, for -ugG")
	fs.BoolVar(&f.user, "u", false, "print only the effective user ID")
	fs.BoolVar(&f.user, "user", false, "print only the effective user ID")
	fs.BoolVar(&f.real, "r", false, "print real ID instead of effective ID")
	fs.BoolVar(&f.real, "real", false, "print real ID instead of effective ID")

	if err := fs.Parse(unixflag.ArgsToGoArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	targetUser := fs.Arg(0)
	if err := run(ioEnv.Stdout, targetUser, f, passwdFile, groupFile); err != nil {
		fmt.Fprintf(ioEnv.Stderr, "%v\n", err)
		return 1
	}
	return 0
}
