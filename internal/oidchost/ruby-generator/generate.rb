require "fileutils"
require "rake"
load File.join(__dir__, "idna.rake")
ENV["VERSION"] = "17.0.0"
ENV["DEST_DIR"] = ENV.fetch("TRB_IDNA_OUTPUT", File.join(__dir__, "..", "ruby/uri/idna/data"))
Rake::Task["idna:generate"].invoke
