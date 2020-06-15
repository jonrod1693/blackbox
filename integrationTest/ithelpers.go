package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/StackExchange/blackbox/v2/pkg/bblog"
	"github.com/StackExchange/blackbox/v2/pkg/bbutil"
	"github.com/StackExchange/blackbox/v2/pkg/vcs"
	_ "github.com/StackExchange/blackbox/v2/pkg/vcs/_all"
	"github.com/TomOnTime/hind"

	"github.com/andreyvit/diff"
)

var verbose = flag.Bool("verbose", false, "reveal stderr")

type userinfo struct {
	name      string
	dir       string // .gnupg-$name
	agentInfo string // GPG_AGENT_INFO
	email     string
	fullname  string
}

var users = map[string]*userinfo{}

func init() {
	testing.Init()
	flag.Parse()
}

var logErr *log.Logger
var logDebug *log.Logger

func init() {
	logErr = bblog.GetErr()
	logDebug = bblog.GetDebug(*verbose)
}

func getVcs(t *testing.T, name string) vcs.Vcs {
	t.Helper()
	// Set up the vcs
	for _, v := range vcs.Catalog {
		logDebug.Printf("Testing vcs: %v == %v", name, v.Name)
		if strings.ToLower(v.Name) == strings.ToLower(name) {
			h, err := v.New()
			if err != nil {
				return nil // No idea how that would happen.
			}
			return h
		}
		logDebug.Println("...Nope.")

	}
	return nil
}

// TestBasicCommands's helpers

func makeHomeDir(t *testing.T, testname string) {
	t.Helper()
	var homedir string
	var err error

	if false {
		// Make a random location that is deleted later
		homedir, err = ioutil.TempDir("", filepath.Join("bbhome-"+testname))
		defer os.RemoveAll(homedir) // clean up
		if err != nil {
			t.Fatal(err)
		}
	} else {
		// Make a predictable location. wipe and re-use
		homedir = "/tmp/bbhome-" + testname
		os.RemoveAll(homedir)
		err = os.Mkdir(homedir, 0o770)
		if err != nil {
			t.Fatal(fmt.Errorf("mk-home %q: %v", homedir, err))
		}
	}

	err = os.Setenv("HOME", homedir)
	if err != nil {
		t.Fatal(err)
	}
	logDebug.Printf("TESTING DIR HOME: cd %v\n", homedir)

	repodir := filepath.Join(homedir, "repo")
	err = os.Mkdir(repodir, 0o770)
	if err != nil {
		t.Fatal(fmt.Errorf("mk-repo %q: %v", repodir, err))
	}
	err = os.Chdir(repodir)
	if err != nil {
		t.Fatal(err)
	}
}

func createDummyFilesAdmin(t *testing.T) {
	// This creates a repo with real data, except any .gpg file
	// is just garbage.
	addLineSorted(t, ".blackbox/blackbox-admins.txt", "user1@example.com")
	addLineSorted(t, ".blackbox/blackbox-admins.txt", "user2@example.com")
	addLineSorted(t, ".blackbox/blackbox-files.txt", "foo.txt")
	addLineSorted(t, ".blackbox/blackbox-files.txt", "bar.txt")
	makeFile(t, "foo.txt", "I am the foo.txt file!")
	makeFile(t, "bar.txt", "I am the foo.txt file!")
	makeFile(t, "foo.txt.gpg", "V nz gur sbb.gkg svyr!")
	makeFile(t, "bar.txt.gpg", "V nz gur one.gkg svyr!")
}

func createFilesStatus(t *testing.T) {
	// This creates a few files with real plaintext but fake cyphertext.
	// There are a variety of timestamps to enable many statuses.
	t.Helper()

	// DECRYPTED: File is decrypted and ready to edit (unknown if it has been edited).
	// ENCRYPTED: GPG file is newer than plaintext. Indicates recented edited then encrypted.
	// SHREDDED: Plaintext is missing.
	// GPGMISSING: The .gpg file is missing. Oops?
	// PLAINERROR: Can't access the plaintext file to determine status.
	// GPGERROR: Can't access .gpg file to determine status.

	addLineSorted(t, ".blackbox/blackbox-files.txt", "status-DECRYPTED.txt")
	addLineSorted(t, ".blackbox/blackbox-files.txt", "status-ENCRYPTED.txt")
	addLineSorted(t, ".blackbox/blackbox-files.txt", "status-SHREDDED.txt")
	addLineSorted(t, ".blackbox/blackbox-files.txt", "status-GPGMISSING.txt")
	// addLineSorted(t, ".blackbox/blackbox-files.txt", "status-PLAINERROR.txt")
	// addLineSorted(t, ".blackbox/blackbox-files.txt", "status-GPGERROR.txt")
	addLineSorted(t, ".blackbox/blackbox-files.txt", "status-BOTHMISSING.txt")

	// Combination of age difference either missing, file error, both missing.
	makeFile(t, "status-DECRYPTED.txt", "File with DECRYPTED in it.")
	makeFile(t, "status-DECRYPTED.txt.gpg", "Svyr jvgu QRPELCGRQ va vg.")

	makeFile(t, "status-ENCRYPTED.txt", "File with ENCRYPTED in it.")
	makeFile(t, "status-ENCRYPTED.txt.gpg", "Svyr jvgu RAPELCGRQ va vg.")

	// Plaintext intentionally missing.
	makeFile(t, "status-SHREDDED.txt.gpg", "Svyr jvgu FUERQQRQ va vg.")

	makeFile(t, "status-GPGMISSING.txt", "File with GPGMISSING in it.")
	// gpg file intentionally missing.

	// Plaintext intentionally missing. ("status-BOTHMISSING.txt")
	// gpg file intentionally missing. ("status-BOTHMISSING.txt.gpg")

	// NB(tlim): commented out.  I can't think of an error I can reproduce.
	// makeFile(t, "status-PLAINERROR.txt", "File with PLAINERROR in it.")
	// makeFile(t, "status-PLAINERROR.txt.gpg", "Svyr jvgu CYNVAREEBE va vg.")
	// setFilePerms(t, "status-PLAINERROR.txt", 0o000)

	// NB(tlim): commented out.  I can't think of an error I can reproduce.
	// makeFile(t, "status-GPGERROR.txt", "File with GPGERROR in it.")
	// makeFile(t, "status-GPGERROR.txt.gpg", "Svyr jvgu TCTREEBE va vg.")
	// setFilePerms(t, "status-GPGERROR.txt.gpg", 0o000)

	time.Sleep(200 * time.Millisecond)

	if err := bbutil.Touch("status-DECRYPTED.txt"); err != nil {
		t.Fatal(err)
	}
	if err := bbutil.Touch("status-ENCRYPTED.txt.gpg"); err != nil {
		t.Fatal(err)
	}
}

func addLineSorted(t *testing.T, filename, line string) {
	err := bbutil.AddLinesToSortedFile(filename, line)
	if err != nil {
		t.Fatalf("addLineSorted failed: %v", err)
	}
}

func makeFile(t *testing.T, name string, lines ...string) {
	t.Helper()

	err := ioutil.WriteFile(name, []byte(strings.Join(lines, "\n")), 0o666)
	if err != nil {
		t.Fatalf("makeFile can't create %q: %v", name, err)
	}
}

func setFilePerms(t *testing.T, name string, perms int) {
	t.Helper()

	err := os.Chmod(name, os.FileMode(perms))
	if err != nil {
		t.Fatalf("setFilePerms can't chmod %q: %v", name, err)
	}
}

var originPath string // CWD when program started.

// checkOutput runs blackbox with args, the last arg is the filename
// of the expected output. Error if output is not expected.
func checkOutput(t *testing.T, args ...string) {
	t.Helper()

	// Pop off the last arg. Use it as the filename.
	name, args := args[hind.G(args)], args[:hind.G(args)]

	cmd := exec.Command(PathToBlackBox(), args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	got, err := cmd.Output()
	if err != nil {
		t.Fatal(fmt.Errorf("checkOutput(%q): %w", args, err))
	}

	want, err := ioutil.ReadFile(filepath.Join(originPath, "test_data", name))
	if err != nil {
		t.Fatalf("checkOutput can't read %v: %v", name, err)
	}

	if w, g := string(want), string(got); w != g {
		t.Errorf("checkOutput(%q) mismatch (-got +want):\n%s",
			args, diff.LineDiff(g, w))
	}

}

func invalidArgs(t *testing.T, args ...string) {
	t.Helper()

	logDebug.Printf("invalidArgs(%q): \n", args)
	cmd := exec.Command(PathToBlackBox(), args...)
	cmd.Stdin = nil
	if *verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if err == nil {
		logDebug.Println("BAD")
		t.Fatal(fmt.Errorf("invalidArgs(%q): wanted failure but got success", args))
	}
	logDebug.Printf("^^^^ (correct error received): err=%q\n", err)
}

// TestAliceAndBob's helpers.

func setupUser(t *testing.T, user, passphrase string) {
	t.Helper()
	logDebug.Printf("DEBUG: setupUser %q %q\n", user, passphrase)
}

var pathToBlackBox string

// PathToBlackBox returns the path to the executable we compile for integration testing.
func PathToBlackBox() string { return pathToBlackBox }

// SetPathToBlackBox sets the path.
func SetPathToBlackBox(n string) {
	logDebug.Printf("PathToBlackBox=%q\n", n)
	pathToBlackBox = n
}

func runBB(t *testing.T, args ...string) {
	t.Helper()

	logDebug.Printf("runBB(%q)\n", args)
	cmd := exec.Command(PathToBlackBox(), args...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		t.Fatal(fmt.Errorf("runBB(%q): %w", args, err))
	}
}

// # NB: This is copied from _blackbox_common.sh
// function get_pubring_path() {
//   : "${KEYRINGDIR:=keyrings/live}" ;
//   if [[ -f "${KEYRINGDIR}/pubring.gpg" ]]; then
//     echo "${KEYRINGDIR}/pubring.gpg"
//   else
//     echo "${KEYRINGDIR}/pubring.kbx"
//   fi
// }

func phase(msg string) {
	logDebug.Println("********************")
	logDebug.Println("********************")
	logDebug.Printf("********* %v\n", msg)
	logDebug.Println("********************")
	logDebug.Println("********************")
}

func makeAdmin(t *testing.T, name, fullname, email string) string {
	testing.Init()

	dir, err := filepath.Abs(filepath.Join(os.Getenv("HOME"), ".gnupg-"+name))
	if err != nil {
		t.Fatal(err)
	}
	os.Mkdir(dir, 0o700)

	u := &userinfo{
		name:     name,
		dir:      dir,
		fullname: fullname,
		email:    email,
	}
	users[name] = u

	// GNUPGHOME=u.dir
	// echo 'pinentry-program' "$(which pinentry-tty)" >> "$GNUPGHOME/gpg-agent.conf"
	os.Setenv("GNUPGHOME", u.dir)
	out, err := bbutil.RunBashOutput("gpg-agent", "--homedir", u.dir, "--daemon")
	if err != nil {
		t.Fatal(err)
	}
	u.agentInfo = strings.TrimSpace(out)

	os.Setenv("GNUPGHOME", u.dir)
	// Generate key:
	bbutil.RunBash("gpg",
		"--homedir", u.dir,
		"--batch",
		"--passphrase", "",
		"--quick-generate-key", u.email,
	)

	return u.dir
}

func become(t *testing.T, name string) {
	testing.Init()
	u := users[name]

	os.Setenv("GNUPGHOME", u.dir)
	os.Setenv("GPG_AGENT_INFO", u.agentInfo)
	bbutil.RunBash("git", "config", "user.name", u.name)
	bbutil.RunBash("git", "config", "user.email", u.fullname)
}

//	// Get fingerprint:
//	// Retrieve fingerprint of generated key.
//	// Use it to extract the secret/public keys.
//	// (stolen from https://raymii.org/s/articles/GPG_noninteractive_batch_sign_trust_and_send_gnupg_keys.html)
//
//	// fpr=`gpg --homedir /tmp/blackbox_createrole --fingerprint --with-colons "$ROLE_NAME" | awk -F: '/fpr:/ {print $10}' | head -n 1`
//	var fpr string
//	bbutil.RunBashOutput("gpg",
//		"--homedir", "/tmp/blackbox_createrole",
//		"--fingerprint",
//		"--with-colons",
//		u.email,
//	)
//	for i, l := range string.Split(out, "\n") {
//		if string.HasPrefix(l, "fpr:") {
//			fpr = strings.Split(l, ":")[9]
//		}
//		break
//	}
//
//	// Create key key:
//	// gpg --homedir "$gpghomedir" --batch --passphrase '' --quick-add-key "$fpr" rsa encr
//	bbutil.RunBash("gpg",
//		"--homedir", u.dir,
//		"--batch",
//		"--passphrase", "",
//		"--quick-add-key", fpr,
//		"rsa", "encr",
//	)

// function md5sum_file() {
//   # Portably generate the MD5 hash of file $1.
//   case $(uname -s) in
//     Darwin | FreeBSD )
//       md5 -r "$1" | awk '{ print $1 }'
//       ;;
//     NetBSD )
//       md5 -q "$1"
//       ;;
//     SunOS )
//       digest -a md5 "$1"
//       ;;
//     Linux )
//       md5sum "$1" | awk '{ print $1 }'
//       ;;
//     CYGWIN* )
//       md5sum "$1" | awk '{ print $1 }'
//       ;;
//     * )
//       echo 'ERROR: Unknown OS. Exiting.'
//       exit 1
//       ;;
//   esac
// }
//
// function assert_file_missing() {
//   if [[ -e "$1" ]]; then
//     echo "ASSERT FAILED: ${1} should not exist."
//     exit 1
//   fi
// }
//
// function assert_file_exists() {
//   if [[ ! -e "$1" ]]; then
//     echo "ASSERT FAILED: ${1} should exist."
//     echo "PWD=$(/usr/bin/env pwd -P)"
//     #echo "LS START"
//     #ls -la
//     #echo "LS END"
//     exit 1
//   fi
// }
// function assert_file_md5hash() {
//   local file="$1"
//   local wanted="$2"
//   assert_file_exists "$file"
//   local found
//   found=$(md5sum_file "$file")
//   if [[ "$wanted" != "$found" ]]; then
//     echo "ASSERT FAILED: $file hash wanted=$wanted found=$found"
//     exit 1
//   fi
// }
// function assert_file_group() {
//   local file="$1"
//   local wanted="$2"
//   local found
//   assert_file_exists "$file"
//
//   case $(uname -s) in
//     Darwin | FreeBSD | NetBSD )
//       found=$(stat -f '%Dg' "$file")
//       ;;
//     Linux | SunOS )
//       found=$(stat -c '%g' "$file")
//       ;;
//     CYGWIN* )
//       echo "ASSERT_FILE_GROUP: Running on Cygwin. Not being tested."
//       return 0
//       ;;
//     * )
//       echo 'ERROR: Unknown OS. Exiting.'
//       exit 1
//       ;;
//   esac
//
//   echo "DEBUG: assert_file_group X${wanted}X vs. X${found}X"
//   echo "DEBUG:" $(which stat)
//   if [[ "$wanted" != "$found" ]]; then
//     echo "ASSERT FAILED: $file chgrp group wanted=$wanted found=$found"
//     exit 1
//   fi
// }
// function assert_file_perm() {
//   local wanted="$1"
//   local file="$2"
//   local found
//   assert_file_exists "$file"
//
//   case $(uname -s) in
//     Darwin | FreeBSD | NetBSD )
//       found=$(stat -f '%Sp' "$file")
//       ;;
//     # NB(tlim): CYGWIN hasn't been tested. It might be more like Darwin.
//     Linux | CYGWIN* | SunOS )
//       found=$(stat -c '%A' "$file")
//       ;;
//     * )
//       echo 'ERROR: Unknown OS. Exiting.'
//       exit 1
//       ;;
//   esac
//
//   echo "DEBUG: assert_file_perm X${wanted}X vs. X${found}X"
//   echo "DEBUG:" $(which stat)
//   if [[ "$wanted" != "$found" ]]; then
//     echo "ASSERT FAILED: $file chgrp perm wanted=$wanted found=$found"
//     exit 1
//   fi
// }
// function assert_line_not_exists() {
//   local target="$1"
//   local file="$2"
//   assert_file_exists "$file"
//   if grep -F -x -s -q >/dev/null "$target" "$file" ; then
//     echo "ASSERT FAILED: line '$target' should not exist in file $file"
//     echo "==== file contents: START $file"
//     cat "$file"
//     echo "==== file contents: END $file"
//     exit 1
//   fi
// }
// function assert_line_exists() {
//   local target="$1"
//   local file="$2"
//   assert_file_exists "$file"
//   if ! grep -F -x -s -q >/dev/null "$target" "$file" ; then
//     echo "ASSERT FAILED: line '$target' should exist in file $file"
//     echo "==== file contents: START $file"
//     cat "$file"
//     echo "==== file contents: END $file"
//     exit 1
//   fi
// }
