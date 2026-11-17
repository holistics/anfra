import recursiveRemoveProperty from './recursive-remove-property.js'
export default function dropFileAdditional(project){
	recursiveRemoveProperty(project,"$file_rel")
	recursiveRemoveProperty(project,"$file_name")
	recursiveRemoveProperty(project,"$file_type")
	}
