RETURN 	= '\n';
SPACE 	= ' ';
TAB 	= '\t';

crdt 	= new Array();
mapping = [];

lastChange = 0;

$(document).ready(function(){
	editor = CodeMirror.fromTextArea(document.getElementById("code"), {
		theme: "dracula",
		matchBrackets: true,
		indentUnit: 4,
		tabSize: 4,
		indentWithTabs: true,
		electricChars: true,
		smartIndent : false,
		mode: "text/x-go"
	});


	// Handles all user inputs before they are applied to the editor.
	editor.on('beforeChange', 
		function(cm, change){
			const curTime = Date.now();

			if (curTime == lastChange) {
				setTimeout(function(){
					lastChange = Date.now();
					handleChange(change);
				}, 1);
			} else {
				lastChange = curTime;
				handleChange(change);
			}
		}
	);

	if (debugMode) {
		// Verifies snippet after processing handle.
		editor.on('change', 
			function(cm, change){
				setTimeout(function(){
					verifyConsistent();
				}, 100);
			}
		);
	}
});

/*
	Object definition for a CRDT element.
*/
class Element {
    constructor(id, prev, next, val, del) {
    	this.id = id;
        this.prev = prev;
        this.next = next;
        this.val = val;
        this.del = del;
    }

    // Set previous element ID
    setPrev(prev) {
		return this.prev = prev;
	}

	// Set next element ID
	setNext(next) {
		return this.next = next;
	}
}

/*
	Dispatches input or delete to 'handleInput' and 'handleRemove'.

	Note: this is very unrefined at the moment, it assumes that the text
		  entered/removed is always no more than one character. 
*/
function handleChange(change) {
	var line = change.from.line;
	var pos  = change.from.ch;

	if (change.origin == "+input") {
		var inputChar;

		// Is this a return case?
		if (change.text.length == 2 && change.text[0] == '' && change.text[1] == '') {
			inputChar = RETURN;
		} // Is this an indent case? 
		else if (change.text[0].includes(TAB) && change.text[0].length > 1) {
			// Break every tab into individual tabs.
			for (var i = 0; i < change.text[0].length; i++) {
				var _pos = 0 + i;

				_.delay(handleInput, i + 1, line, _pos, TAB);
			}

			return;
		} // Some weird CodeMirror shit
		else if (change.text[0] == '') {
			return;
		} // Is this every other case? 
		else {
			inputChar = change.text[0];
		}

		handleInput(line, pos, inputChar);
	} else if (change.origin == '+delete') {
		// TODO deal with block deletion, or at least find a way to avoid it

		handleRemove(line, pos);
	}
}

/**
	Handles input events.
*/
function handleInput(line, pos, val) {
	var newLine = false;

	// Assign an ID
	const id = getID();

	if (mapping[line] === undefined) mapping.push([]);

	var prevElem, nextElem, prev, next;
	prevElem = getPrevElem(line, pos);
	if (prevElem !== undefined) {
		prev = prevElem.id;
		next = prevElem.next;

		prevElem.next = id;
	} else {
		next = mapping[line][pos];
	}

	if (next !== undefined) {
		nextElem = crdt[next];

		nextElem.prev = id;
	}

	// Update CRDT
	const elem = new Element(id, prev, next, val, false);
	crdt[id] = elem;

	// Update the mapping.
	const _line = mapping[line];
	const thisElem = crdt[_line[pos]];

	// If an element exists at this (line, pos), its either a 
	// carriage return or any other type of character.
	if (thisElem !== undefined && val == RETURN) {

		// Add a new line right after this one.
		mapping.splice(line + 1, 0, []);

		// Move all elements beyong this position down.
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

/**
	Set the CRDT element to deleted and removes the mapping;
*/
function handleRemove(line, pos) {
	const id = mapping[line][pos];
	const elem = crdt[id];

	if (elem === undefined) return;

	elem.del = true;
	if (mapping[line].length > 0) {
		mapping[line].splice(pos, 1);
	} else {
		delete mapping[line];
	}

	if (debugMode) {
		console.log("Observed remove at line: " + line + " pos: " + pos);
	}
}

/**
	Get the previous element based on te insertion at the line and pos.
*/
function getPrevElem(line, pos) {
	var prev = undefined;

	if (line == 0 && pos == 0) {
		// Start of snippet: undefined.
	} else if (pos > 0) {
		prev = mapping[line][pos - 1];
	} else if (line > 0) {
		const _line = mapping[line-1];

		prev = _line[_line.length-1];
	}

	return crdt[prev];
}

/**
	Get the next element based on te insertion at the line and pos.

	Assumes the current element is already in the mapping.
*/
function getNextElem(line, pos) {
	var next = undefined;

	const _line = mapping[line];
	if (_line.length > 0 && _line[pos + 1] !== undefined) {
		next = _line[pos + 1];
	} else if (_line[pos + 1] == undefined && mapping[line+1] !== undefined) {
		next = mapping[line + 1][0];
	} else {
		if (mapping[line+1] !== undefined) {
			next = mapping[line+1][0];
		}
	}
	
	return crdt[next];
}

/**
	Converts the CRDT to a 2D mapping array.
*/
function crdtToMapping(_crdt) {
	var _mapping = [];

	var curElem = getStartElem();
	var lastVal, lastPos, lastLine;
	while (curElem != undefined) {
		if (curElem.del !== true) {
			var curLine, curPos;
			if (lastLine == undefined) {
				_mapping.push([]);
				curLine = 0;
			} else if (lastVal == RETURN) {
				_mapping.push([]);
				curLine = lastLine + 1;
			}

			if (lastPos == undefined || lastVal == RETURN) { 
				curPos = 0;
			} else {
				curPos = lastPos + 1;
			}

			_mapping[curLine][curPos] = curElem.id;

			lastVal = curElem.val;
			lastPos = curPos;
			lastLine = curLine;
		}

		curElem = _crdt[curElem.next];
	}

	return _mapping;
}

/**
	Converts a 2D mapping array with its associated crdt to a string.
*/
function mappingToSnippet(_mapping, _crdt) {
	var snippet = "";

	_mapping.forEach(function(line){
		line.forEach(function(id){
			snippet = snippet + _crdt[id].val;
		});
	});

	return snippet;
}

/**
	Converts a CRDT to a string.
*/
function crdtToSnippet(_crdt) {
	var mapping = crdtToMapping(_crdt);
	var snippet = mappingToSnippet(mapping, _crdt);

	return snippet;
}

/**
	Gets the first element of the sequence CRDT.
*/
function getStartElem() {
	var start = null;

	const ids = Object.keys(crdt);
	$(ids).each(function(index, id){
		const elem = crdt[id];
		if (elem.prev == undefined) {
			start = elem;
			return;
		}
	});

	return start;
}

/**
	Checks if the CRDT matches the editors value.
*/
function verifyConsistent() {
	var snippet = editor.getValue();
	var _snippet = crdtToSnippet(crdt);

	if (snippet != _snippet) {
		console.error('Snippet is not consistent!\n' + 'In editor: \n' + snippet + '\nFrom CRDT:\n' + _snippet);
		return false;
	} else {
		return true;
	}
}

/**
	Create UID based on the current time and userID.
*/
function getID() {
	var id = userID + '_' + Date.now() + '_0';

	const elem = crdt[id];
	if (elem !== undefined) {
		var i = 1;
		while (crdt[id + '_' + i] !== undefined) {
			i++;
		}

		id = id + '_' + i;
	}

	return id;
}

/**
	Converts a mapping line (array of IDs) to an array
	of values from the CRDT.
*/
function mappingLineToValArray(line) {
	var valArray = [];

	line.forEach(function(id, i){
		valArray[i] = crdt[id].val;
	});

	return valArray;
}

/**
	Replaces all IDs with values.
*/
function getValMapping(_mapping) {
	var valMapping = [];

	_mapping.forEach(function(line, i){
		valMapping.push([]);
		valMapping[i] = mappingLineToValArray(line);
	});

	return valMapping;
}

/**
	Returns array without leading white space.
*/
function stripLeadingWhitespace(line) {
	var arr = [];
	var nextIndex = 0;

	mapping[line].forEach(function(id, i){
		const elem = crdt[id];
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


