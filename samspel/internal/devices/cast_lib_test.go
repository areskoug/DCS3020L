package devices

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/cast"
	pb "github.com/vishen/go-chromecast/cast/proto"
)

// fakeConn stands in for a Chromecast. Every request it receives is answered at once with a receiver status
// that carries the same request id, which is how a fast device behaves.
type fakeConn struct {
	msgs   chan *pb.CastMessage
	closed atomic.Bool
}

func newFakeConn() *fakeConn { return &fakeConn{msgs: make(chan *pb.CastMessage, 64)} }

func (f *fakeConn) message(requestID int) *pb.CastMessage {
	payload := fmt.Sprintf(`{"type":"RECEIVER_STATUS","requestId":%d,"status":{"applications":[],"volume":{"level":0.5,"muted":false}}}`, requestID)
	return &pb.CastMessage{PayloadUtf8: &payload}
}

func (f *fakeConn) Start(string, int) error       { return nil }
func (f *fakeConn) MsgChan() chan *pb.CastMessage { return f.msgs }
func (f *fakeConn) SetDebug(bool)                 {}
func (f *fakeConn) LocalAddr() (string, error)    { return "127.0.0.1", nil }
func (f *fakeConn) RemoteAddr() (string, error)   { return "127.0.0.1", nil }
func (f *fakeConn) RemotePort() (string, error)   { return "8009", nil }
func (f *fakeConn) Close() error {
	f.closed.Store(true)
	return nil
}
func (f *fakeConn) Send(requestID int, _ cast.Payload, _, _, _ string) error {
	if !f.closed.Load() {
		select {
		case f.msgs <- f.message(requestID):
		default:
		}
	}
	return nil
}

// Closing an application while the device is still sending used to panic with "send on closed channel",
// which took the whole hub down.
func TestClosingWhileTheDeviceStillTalksDoesNotPanic(t *testing.T) {
	for i := 0; i < 300; i++ {
		c := newFakeConn()
		app := application.NewApplication(application.WithConnection(c), application.WithCacheDisabled(true))
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case c.msgs <- c.message(0):
				case <-stop:
					return
				}
			}
		}()
		time.Sleep(time.Duration(i%7) * 100 * time.Microsecond)
		app.Close(false)
		close(stop)
		wg.Wait()
	}
}

// Several connections starting at once, as when a group is started or the phone changes the volume, must
// not trip over shared state, and a quick answer must not be lost.
func TestStartingManyConnectionsAtOnce(t *testing.T) {
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := newFakeConn()
			app := application.NewApplication(application.WithConnection(c), application.WithCacheDisabled(true))
			defer app.Close(false)
			done := make(chan error, 1)
			go func() { done <- app.Start("127.0.0.1", 8009) }()
			select {
			case err := <-done:
				if err != nil {
					errs <- err
				}
			case <-time.After(2 * time.Second):
				errs <- fmt.Errorf("Start waited for an answer that had already arrived")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
