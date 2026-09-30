package cli

import "testing"

func TestJSONCodecUsesDeclarationRootsAcrossEmissionGroups(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"values/status.trb": "enum Status\n\tReady = \"ready\"\nend\n",
		"contracts/payload.trb": `import values/status

record Payload
	status: Status
end
`,
		"main.trb": `import values/status
import contracts/payload
import trb/std/json

def main()
	encoded := JSON.encode(Payload.new(status: Status::Ready)) catch |_error|
		return
	end
	decoded := JSON.decode<Payload>(encoded) catch |_error|
		return
	end
	puts(decoded.status.raw_value())
	return
end
`,
	}, "ready\n", "")
}
