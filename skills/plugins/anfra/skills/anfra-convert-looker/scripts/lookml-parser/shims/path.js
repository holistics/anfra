// vendor: the posix `basename` and `dirname` of the node path module, for the
// relative "/" paths of LookML project files.

function trimTrailingSlashes(p) {
	let end = p.length
	while (end > 1 && p[end - 1] === '/') end -= 1
	return p.slice(0, end)
}

function basename(p) {
	const trimmed = trimTrailingSlashes(p)
	return trimmed === '/' ? '' : trimmed.slice(trimmed.lastIndexOf('/') + 1)
}

function dirname(p) {
	const trimmed = trimTrailingSlashes(p)
	const index = trimmed.lastIndexOf('/')
	if (index === -1) return '.'
	if (index === 0) return '/'
	return trimTrailingSlashes(trimmed.slice(0, index))
}

export default { basename, dirname }
