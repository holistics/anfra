import encodeProperty from './encode-property.js';

export default function indexBy(key){
	return (typeof key=="function"
		? (obj,x,i) => ({...obj, [encodeProperty(key(x))]:x})
		: (obj,x,i) => ({...obj, [encodeProperty(x[key])]:x})
		)
	}
