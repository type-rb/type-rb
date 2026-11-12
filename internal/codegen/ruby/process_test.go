//go:build darwin || linux

package ruby

import (
	"os/exec"
	"testing"
)

func TestProcessAdapterCleanup(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby unavailable")
	}
	source := rubyProcessRuntime + `
TrbProcessRuntime.capture("/bin/sh", ["-c", "exit 0"])
before = Dir.children("/dev/fd").length
12.times do
  stdout, _stderr, status = TrbProcessRuntime.capture("/bin/sh", ["-c", "printf $$"])
  pid = Integer(stdout)
  raise "missing child identity" unless status.success? && pid > 0
  begin
    Process.kill(0, pid)
    raise "owned child still exists"
  rescue Errno::ESRCH
  end
  begin
    TrbProcessRuntime.capture("/type-rb-process-does-not-exist", [])
    raise "missing launch error"
  rescue IOError => error
    raise unless error.message.include?("No such file")
  end
end
raise "process descriptors leaked" unless Dir.children("/dev/fd").length == before
puts "joined and closed"
`
	if output, err := exec.Command("ruby", "-e", source).CombinedOutput(); err != nil || string(output) != "joined and closed\n" {
		t.Fatalf("cleanup: %v\n%s", err, output)
	}
}
