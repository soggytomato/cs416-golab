/* =============================================================================
									CRDT MAPPING
   =============================================================================*/

// Global var for the local mapping from the text area characters
// to their associated CRDT elements. 
// Note: this does NOT hold tombstones, only non-deleted elements.
mapping = [];

/*
	Updates the mapping at the given line and pos
	with provided value.
*/
function updateMapping(line, pos, id) {
	if (mapping[line] === undefined) mapping.push([]);

	const val = CRDT.get(id).val;
	const _line = mapping[line];
	const thisElem = CRDT.get(_line[pos]);

	// If an element exists at this (line, pos), its either a 
	// carriage return or any other type of character.
	if (thisElem !== undefined && val == RETURN) {

		// Add a new line right after this one.
		mapping.splice(line + 1, 0, []);

		// Move all elements beyond this position down.
		const chars = _line.splice(pos, _line.length - pos);
		mapping[line + 1] = chars;
		stripLeadingWhitespace(line+1);
	}

	// Update mapping
	mapping[line].splice(pos, 0, id);

	if (debugMode) {
		console.log("Observed input at line: " + line + " pos: " + pos + " char: " + unescape(val));		
	}
}
