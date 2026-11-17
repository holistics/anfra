// vendor: the part of bluebird that parse-files uses. Files are in memory, so
// the concurrency limit has no effect and is ignored. Order is kept.

function map(items, mapper) {
	return Promise.all(Array.from(items, (item, index) => mapper(item, index)))
}

export default { map }
