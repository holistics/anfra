import assembleModels from './assemble-models.js'
import dropStrings from './drop-strings.js'
import dropFileAdditional from './drop-file-additional.js'
import recursiveRemoveProperty from './recursive-remove-property.js'
import applyExtensionsRefinements from './apply-extensions-refinements.js'
import removeAbstract from './remove-abstract.js'
import addPositions from './add-positions.js'

const byCliFlags = {
	'm': assembleModels,
	's': dropStrings,
	'x': (project) => { applyExtensionsRefinements(project); removeAbstract(project) },
	'f': dropFileAdditional,
	'p': addPositions,
	'a': (project) => project
}

export {
	byCliFlags,
	assembleModels,
	applyExtensionsRefinements,
	removeAbstract,
	dropStrings,
	dropFileAdditional,
	recursiveRemoveProperty,
	addPositions
}

