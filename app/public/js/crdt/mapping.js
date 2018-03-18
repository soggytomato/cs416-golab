/* =============================================================================
							MAPPING CLASS DEFINITION
   =============================================================================*/
/*
	A Mapping is a record of all non-deleted elements' IDs present in the snippet
	mapped to their respective locations in the snippet.

	For instance, a snippet as follows:
	"abc
	 xyz"

	would have two lines, where the first line contains the element IDs
	for "abc\n" and the second line "xyz". The object would be as such:
		[ 	[id1, id2, id3]
			[id4, id5]		]

	Once an element is deleted, it is removed from the mapping.
*/
class Mapping {
	constructor() {
    	this.arr = [];
    }

    // Line + Pos Functions

    get(line, pos) {
    	return this.arr[line][pos];
    }

    set(line, pos, id) {
    	this.arr[line].splice(pos, 0, id);
    }

    delete(line, pos) {
    	this.arr[line].splice(pos, 1);
    }

    length() {
    	return this.arr.length;
    }

    // Line Functions

    getLine(line) {
    	return this.arr[line];
    }

    setLine(line, _line) {
    	this.arr[line] = _line;
    }

    addLine() {
    	this.arr.push([]);
    }

    insertLine(line) {
		this.arr.splice(line, 0, []);
    }

    deleteLine(line) {
    	this.arr.splice(line, 1);
    }

    lineLength(line) {
    	return this.arr[line].length;
    }

    getLines() {
    	return this.arr;
    }

    // Other Functions

    /*
	Updates the mapping at the given line and pos with provided value.*/
    update(line, pos, id) {
		if (mapping.getLine(line) === undefined) mapping.addLine();

		const val = CRDT.get(id).val;
		const _line = mapping.getLine(line);
		const thisElem = CRDT.get(_line[pos]);

		// If an element exists at this (line, pos), its either a 
		// carriage return or any other type of character.
		if (thisElem !== undefined && val == RETURN) {

			// Add a new line right after this one.
			mapping.insertLine(line + 1);

			// Move all elements beyond this position down.
			const chars = _line.splice(pos, _line.length - pos);
			mapping.setLine(line + 1, chars);
			mapping.stripWhitespace(line+1);
		}

		// Update mapping
		mapping.set(line, pos, id);

		if (debugMode) {
			console.log("Observed input at line: " + line + " pos: " + pos + " char: " + unescape(val));		
		}
	}

	/*
	Get the previous element based on a line and position.*/
	getPreceding(line, pos) {
		var prev = undefined;

		if (line == 0 && pos == 0) {
			// Start of snippet: undefined.
		} else if (pos > 0) {
			prev = mapping.get(line, pos - 1);
		} else if (line > 0) {
			const _line = mapping.getLine(line-1);

			prev = _line[_line.length-1];
		}

		return prev;
	}

	/*
	Removes all whitespace at the beginning of the lines. */
	stripWhitespace(line) {
		var arr = [];
		var nextIndex = 0;

		mapping.getLine(line).forEach(function(id, i){
			const elem = CRDT.get(id);
			const val = elem.val;
			if (val.trim().length > 0 || val == RETURN || val == SPACE) {
				arr[nextIndex] = id;
				nextIndex++;
			} else {
				elem.del = true;
			}
		});

		mapping.setLine(line, arr);
	}

	/*
	Converts a 2D mapping array with its associated CRDT to a string.*/
	toSnippet() {
		var snippet = "";

		this.getLines().forEach(function(line){
			line.forEach(function(id){
				snippet = snippet + CRDT.get(id).val;
			});
		});

		return snippet;
	}

	/*
	Return a mapping with values instead of element IDs.*/
	toVals() {
		var _mapping = [];

		this.getLines().forEach(function(line, i){
			_mapping.push([]);

			line.forEach(function(id, j){
				_mapping[i][j] = CRDT.get(id).val;
			});
		});

		return _mapping;
	}
}
mapping = new Mapping();
