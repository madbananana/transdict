// Package instance keeps exactly one copy of Transdict running.
//
// Two copies is not a cosmetic problem: both register the same global
// shortcuts, so one keypress queries twice, and both write the same config
// file, so whichever settings page closes last decides what the other one
// believes. An operating-system file lock arbitrates because it disappears
// with the process — a crashed instance cannot leave the app unlaunchable.
package instance

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"transdict/internal/appdir"
)

// ErrBusy means another running copy holds the lock.
var ErrBusy = errors.New("已有另一个 Transdict 在运行")

// errLocked separates "someone else has it" from a real failure to open the
// lock file, which the caller should log rather than treat as a rival process.
var errLocked = errors.New("instance: locked")

const (
	lockName = "instance.lock"
	sockName = "instance.sock"

	// A wake request is one short word over a local socket. Anything slower
	// than this means the primary is wedged, and waiting longer will not help.
	notifyTimeout = 600 * time.Millisecond
)

// Lock is the held lock. Keep it for the lifetime of the process: exit
// releases both the file lock and the socket, so a running app has nothing to
// clean up. Close exists for tests and for an orderly shutdown.
type Lock struct {
	file *os.File
	ln   net.Listener
	cmds chan string
}

// Commands delivers the requests made by later launches. The names are the
// ones the tray already uses: main, input, settings.
func (l *Lock) Commands() <-chan string { return l.cmds }

// Acquire takes the single-instance lock and starts serving wake requests.
func Acquire() (*Lock, error) {
	dir, err := appdir.Root()
	if err != nil {
		return nil, err
	}
	return acquire(dir)
}

func acquire(dir string) (*Lock, error) {
	file, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := hold(file); err != nil {
		file.Close()
		if errors.Is(err, errLocked) {
			return nil, ErrBusy
		}
		return nil, err
	}

	l := &Lock{file: file, cmds: make(chan string, 8)}

	// Holding the lock is the proof that whoever created this socket file is
	// gone, so removing it cannot race a live peer.
	sock := filepath.Join(dir, sockName)
	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		// Keep the lock anyway: a second launch that finds it and does nothing
		// beats two copies because the local pipe could not be created.
		log.Printf("transdict: 无法监听唤醒请求: %v", err)
	} else {
		l.ln = ln
		go l.serve()
	}
	return l, nil
}

// Close releases the lock.
func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	if l.ln != nil {
		l.ln.Close()
	}
	err := release(l.file)
	l.file.Close()
	return err
}

func (l *Lock) serve() {
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return
		}
		go answer(conn, l.cmds)
	}
}

// answer reads one command and acknowledges it, so the requester can tell a
// primary that woke up from one that accepted the connection and then froze.
//
// The command is newline-terminated because a zero-length write is not
// observable on a socket: an empty request would otherwise sit unread until
// both sides gave up.
func answer(conn net.Conn, cmds chan string) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(notifyTimeout))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && line == "" {
		return
	}
	cmd := strings.TrimSpace(line)
	if cmd == "" {
		cmd = "main"
	}
	// A full channel means the UI loop is not draining commands; dropping the
	// request is better than blocking the accept loop forever.
	select {
	case cmds <- cmd:
	default:
	}
	io.WriteString(conn, "ok")
}

// Notify asks the running copy to surface itself. It is best-effort: the
// caller exits either way, so a failure is a log line rather than something
// the user can act on.
func Notify(cmd string) error {
	dir, err := appdir.Root()
	if err != nil {
		return err
	}
	return notify(dir, cmd)
}

func notify(dir, cmd string) error {
	conn, err := net.DialTimeout("unix", filepath.Join(dir, sockName), notifyTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(notifyTimeout))
	if _, err := io.WriteString(conn, cmd+"\n"); err != nil {
		return err
	}
	reply, err := io.ReadAll(io.LimitReader(conn, 8))
	if err != nil {
		return err
	}
	if string(reply) != "ok" {
		return errors.New("instance: 应答异常: " + string(reply))
	}
	return nil
}
