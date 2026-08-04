package stdlib

func pathSource() string {
	return `class Path
	def self.separator(): String
		return "/"
	end

	def self.clean(value: String): String
		if value == ""
			return "."
		end
		absolute := value.start_with?("/")
		parts := value.split("/")
		mut normalized: Array<String> := []
		parts.each do |part|
			if part == "" or part == "."
				next
			end
			if part == ".."
				if absolute
					if not normalized.empty?()
						normalized.pop()
					end
				elsif normalized.empty?() or normalized.last() == ".."
					normalized.push("..")
				else
					normalized.pop()
				end
			else
				normalized.push(part)
			end
		end
		joined := normalized.join("/")
		if absolute
			if joined == ""
				return "/"
			end
			return "/" + joined
		end
		if joined == ""
			return "."
		end
		return joined
	end

	def self.join(left: String, right: String): String
		if left == ""
			return Path.clean(right)
		end
		if right == ""
			return Path.clean(left)
		end
		return Path.clean(left + "/" + right)
	end

	def self.absolute(value: String): Boolean
		return value.start_with?("/")
	end

	def self.components(value: String): Array<String>
		cleaned := Path.clean(value)
		mut result: Array<String> := []
		if cleaned == "." or cleaned == "/"
			return result
		end
		cleaned.split("/").each do |part|
			if part != ""
				result.push(part)
			end
		end
		return result
	end

	def self.base(value: String): String
		cleaned := Path.clean(value)
		if cleaned == "/" or cleaned == "."
			return cleaned
		end
		return Path.components(cleaned).last()
	end

	def self.directory(value: String): String
		cleaned := Path.clean(value)
		if cleaned == "/" or cleaned == "."
			return cleaned
		end
		absolute := Path.absolute(cleaned)
		mut parts := Path.components(cleaned)
		parts.pop()
		if parts.empty?()
			if absolute
				return "/"
			end
			return "."
		end
		joined := parts.join("/")
		if absolute
			return "/" + joined
		end
		return joined
	end
end

def separator(): String
	return Path.separator()
end

def clean(value: String): String
	return Path.clean(value)
end

def join(left: String, right: String): String
	return Path.join(left, right)
end

def absolute(value: String): Boolean
	return Path.absolute(value)
end

def components(value: String): Array<String>
	return Path.components(value)
end

def base(value: String): String
	return Path.base(value)
end

def directory(value: String): String
	return Path.directory(value)
end
`
}
