package cli

import "testing"

func TestClassAliasConstructionAcrossTargetsAndREPL(t *testing.T) {
	runPortableExecutionCase(t, `class Box
  @value: Integer
  def initialize(value: Integer)
    @value = value
    return
  end
  def self.constant(): Integer
    return 5
  end
end
alias Wrapped = Box
def main()
  puts(Wrapped.new(14).value)
  puts(Wrapped.constant())
end
`, "14\n5\n", "")
}
