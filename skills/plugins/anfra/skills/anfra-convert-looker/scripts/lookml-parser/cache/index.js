// vendor: a no-op stub of the upstream file cache. The upstream cache keeps
// parsed files on disk and hashes them with fs and crypto. Without a hash,
// parse-files does not write to the cache.

class FileCache {
	get() { return undefined }
	getByHash() { return undefined }
	set() {}
	saveDiskCache() {}
}

function hashContent() {
	return undefined
}

export {
	FileCache,
	hashContent
}
