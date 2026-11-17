// vendor: only the in-memory branch of the upstream loader. The glob and git
// loaders need fs, glob and child_process, which a browser does not have.
// The host reads the files and gives them as `input: [{ path, content }]`.

async function loadFileObjects(options = {}) {
	const {
		input,
		source,
		gitRef
	} = options

	const inputPattern = input !== undefined ? input : source

	if (Array.isArray(inputPattern)) {
		if (gitRef) {
			const err = new Error("Cannot specify both an in-memory file array and gitRef")
			err.code = "INVALID_OPTIONS"
			throw err
		}
		return {
			fileObjects: inputPattern.map(s => ({
				path: s.path,
				read: () => Promise.resolve(s.content)
			}))
		}
	}

	const err = new Error("This copy of lookml-parser reads only an in-memory file array: input: [{ path, content }]")
	err.code = "INVALID_OPTIONS"
	throw err
}

export {
	loadFileObjects
}
