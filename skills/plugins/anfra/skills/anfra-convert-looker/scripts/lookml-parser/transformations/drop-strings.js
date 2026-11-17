import recursiveRemoveProperty from './recursive-remove-property.js'
export default function dropStrings(project){
	return recursiveRemoveProperty(project,"$strings")
	}