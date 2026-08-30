package stdlib

func filesystemSource() string {
	return `import trb/std/result
import trb/std/unit
import trb/internal/filesystem as native_fs

module FileSystem
	enum ErrorKind
		Other
		AlreadyExists
		InvalidLimit
		TooLarge
	end

	record Error
		operation: String
		path: String
		message: String
		kind: FileSystem::ErrorKind = FileSystem::ErrorKind::Other
	end

	enum OpenMode
		Read
		Write
		CreateNew
	end

	enum DirectoryEntryKind
		File
		Directory
		Other
	end

	record DirectoryEntry
		name: String
		kind: FileSystem::DirectoryEntryKind
	end

	class File
	end
end

def exists(path: String): Result<Boolean, FileSystem::Error>
	return native_fs.exists(path)
end

def read_text(path: String): Result<String, FileSystem::Error>
	return native_fs.read_text(path)
end

def read_bytes(path: String): Result<Bytes, FileSystem::Error>
	return native_fs.read_bytes(path)
end

def write_text(path: String, value: String): Result<Unit, FileSystem::Error>
	return native_fs.write_text(path, value)
end

def write_bytes(path: String, value: Bytes): Result<Unit, FileSystem::Error>
	return native_fs.write_bytes(path, value)
end

def create_directory(path: String): Result<Unit, FileSystem::Error>
	return native_fs.create_directory(path)
end

def list(path: String): Result<Array<String>, FileSystem::Error>
	return native_fs.list(path)
end
`
}
