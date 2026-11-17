// vendor: the yaml package, not js-yaml. Loaded lazily, so that LookML (.lkml) files parse without it installed;
// only YAML dashboards (.lookml) need it.
const parseYaml = await import('yaml').then(m => m.parse, () => () => { throw new Error("the 'yaml' package is not installed") })
let jsYaml
// let priorDependencyError

export default function maybeYamlParse(stringToParse){
	try{
		jsYaml = { load: (s) => parseYaml(s, { merge: true }) } // vendor: merge: true keeps the "<<" merge keys of js-yaml
		const result = jsYaml.load(stringToParse)
		return result
		}
	catch(e){
		if(e.message.includes()){
			throw e.message+". js-yaml is an optional dependency, and must be explicitly installed to parse yaml-based files, such as LookML Dashboards."
			}
		throw e.message
		}
	}
