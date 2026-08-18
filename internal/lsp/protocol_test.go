package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"
)

func TestRPCStreamSerializesConcurrentFrames(t *testing.T) {
	output := &yieldingBuffer{}
	stream := newRPCStream(bytes.NewReader(nil), output)
	const frameCount = 64
	var writers sync.WaitGroup
	for index := 0; index < frameCount; index++ {
		index := index
		writers.Add(1)
		go func() {
			defer writers.Done()
			if index%2 == 0 {
				if err := stream.write(success(json.RawMessage(fmt.Sprint(index)), index)); err != nil {
					t.Errorf("response write: %v", err)
				}
				return
			}
			if err := stream.write(notification{JSONRPC: "2.0", Method: fmt.Sprintf("test/%d", index), Params: index}); err != nil {
				t.Errorf("notification write: %v", err)
			}
		}()
	}
	writers.Wait()

	frames := decodeFrames(t, output.Bytes())
	if len(frames) != frameCount {
		t.Fatalf("decoded frames=%d, want %d", len(frames), frameCount)
	}
	seen := make(map[string]bool, frameCount)
	for _, frame := range frames {
		if id := string(frame["id"]); id != "" {
			seen["id:"+id] = true
			continue
		}
		var method string
		if err := json.Unmarshal(frame["method"], &method); err != nil {
			t.Fatalf("decode method: %v", err)
		}
		seen["method:"+method] = true
	}
	for index := 0; index < frameCount; index++ {
		key := fmt.Sprintf("method:test/%d", index)
		if index%2 == 0 {
			key = fmt.Sprintf("id:%d", index)
		}
		if !seen[key] {
			t.Errorf("missing frame %s", key)
		}
	}
}

type yieldingBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *yieldingBuffer) Write(contents []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, value := range contents {
		if err := b.buffer.WriteByte(value); err != nil {
			return 0, err
		}
		runtime.Gosched()
	}
	return len(contents), nil
}

func (b *yieldingBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buffer.Bytes()...)
}
