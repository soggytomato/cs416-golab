/* =============================================================================
									UTILITIES
   =============================================================================*/

/*
	Checks if the current sequence CRDT matches the editors value. 
*/
function verifyConsistent() {
	var snippet = editor.getValue();
	var _snippet = CRDT.toSnippet();

	if (snippet != _snippet) {
		alert('CRDT and editor fell out of sync! Check console for details.');
		console.error('Snippet is not consistent!\n' + 'In editor: \n' + snippet + '\nFrom CRDT:\n' + _snippet);

		return false;
	} else {
		return true;
	}
}

/*
	Returns array without leading white space.
*/
function stripLeadingWhitespace(line) {
	var arr = [];
	var nextIndex = 0;

	mapping[line].forEach(function(id, i){
		const elem = CRDT.get(id);
		const val = elem.val;
		if (val.trim().length > 0 || val == RETURN || val == SPACE) {
			arr[nextIndex] = id;
			nextIndex++;
		} else {
			elem.del = true;
		}
	});

	mapping[line] = arr;
}

/**
	Converts a 2D mapping array with its associated CRDT to a string.
*/
function mappingToSnippet(_CRDT = CRDT, _mapping = mapping) {
	var snippet = "";

	_mapping.forEach(function(line){
		line.forEach(function(id){
			snippet = snippet + CRDT.get(id).val;
		});
	});

	return snippet;
}

/**
	Converts a mapping line to an array of values from the CRDT.
*/
function mappingLineToValArray(line) {
	var valArray = [];

	line.forEach(function(id, i){
		valArray[i] = CRDT.get(id).val;
	});

	return valArray;
}

/**
	Replaces all IDs with values.
*/
function getValMapping(_mapping = mapping) {
	var valMapping = [];

	_mapping.forEach(function(line, i){
		valMapping.push([]);
		valMapping[i] = mappingLineToValArray(line);
	});

	return valMapping;
}
