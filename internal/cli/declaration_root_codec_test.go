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

func TestJSONCodecKeepsTransitiveEnumIdentityAcrossBackends(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"models.trb": `enum Status
	Remote = "REMOTE"
end
record User
	status: Status
end
`,
		"main.trb": `import { User } from models
import trb/std/json
enum Status
	Local = "LOCAL"
end
def main()
	decoded := JSON.decode<User>("{\"status\":\"REMOTE\"}") catch |error|
		puts(error.message)
		return
	end
	puts(decoded.status.raw_value())
	puts(Status::Local.raw_value())
	encoded := JSON.encode(decoded) catch |error|
		puts(error.message)
		return
	end
	puts(encoded)
	return
end
`,
	}, "REMOTE\nLOCAL\n{\"status\":\"REMOTE\"}\n", "")
}
