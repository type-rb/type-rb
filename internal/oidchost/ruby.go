// Package oidchost holds the qualified, compiler-owned OIDC hostname support.
package oidchost

import (
	"embed"
	"path"
	"strings"
)

//go:embed ruby
var rubyFiles embed.FS

// Ruby emits the pinned provider in dependency order. Only OIDC applications
// include it; generated Ruby does not require a native library or a Ruby gem.
func Ruby() string {
	var output strings.Builder
	for _, name := range []string{"ruby/LICENSE.txt", "ruby/UNICODE-LICENSE.txt"} {
		license, err := rubyFiles.ReadFile(name)
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(license), "\n") {
			output.WriteString("# " + line + "\n")
		}
	}
	seen := map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		source, err := rubyFiles.ReadFile(name)
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(source), "\n") {
			if strings.HasPrefix(line, "require_relative ") {
				relative := strings.Trim(strings.TrimPrefix(line, "require_relative "), "\"")
				visit(path.Join(path.Dir(name), relative+".rb"))
			} else {
				if line == "module URI" {
					line = "module TrbOidcIdna"
				}
				output.WriteString(line + "\n")
			}
		}
	}
	visit("ruby/uri/idna.rb")
	output.WriteString(`
module TrbOidcIdna
  def self.to_ascii(host)
    IDNA.to_ascii(host, use_std3_ascii_rules: true, check_hyphens: true,
      check_bidi: true, check_joiners: true, transitional_processing: false,
      verify_dns_length: true, ignore_invalid_punycode: false)
  end
end
`)
	return output.String()
}
