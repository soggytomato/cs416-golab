/* =============================================================================
								OPERATION HANDLERS
   =============================================================================*/

// String constants
RETURN 	= '\n';
SPACE 	= ' ';
TAB 	= '\t';
EMPTY 	= '';

// Operation constants
INPUT_OP = '+input';
DELETE_OP = '+delete';
REMOTE_INPUT_OP = '+remote_input';
REMOTE_DELETE_OP = '+remote_delete';
REMOTE_INPUT_OP_PREFIX = REMOTE_INPUT_OP + '_';
REMOTE_DELETE_OP_PREFIX = REMOTE_DELETE_OP + '_';

// Timestamp for the last encountered operation
lastChange = 0;

/*
	Register the handlers for events coming from CodeMirror.

	Everytime there is an operation (key stroke), it will first hit the
	'beforeChange' event handler which will process the operation before
	the key stroke is actually applied to the text area that the user sees.

	The 'change' event occurs after the operation has been processed and
	added to the text area. At this point, we check consistency 
	between the editors text area and the CRDT, given we are in debug mode.
*/
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
					handleOperation(change);
				}, 1);
			} else {
				lastChange = curTime;
				handleOperation(change);
			}
		}
	);

	// Handles all user inputs after they are applied to the editor.
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
	Dispatches input or delete to 'handleInput' and 'handleRemove'.

	Note: this is very unrefined at the moment, it assumes that the text
		  entered/removed is always no more than one 'character'. 
*/
function handleOperation(op) {
	var line = op.from.line;
	var pos  = op.from.ch;

	const origin = op.origin;
	if (origin == INPUT_OP) {
		var inputChar;

		// Is this a return case?
		if (op.text.length == 2 && op.text[0] == EMPTY && op.text[1] == EMPTY) {
			inputChar = RETURN;
		} // Is this an indent case? 
		else if (op.text[0].includes(TAB) && op.text[0].length > 1) {
			// Break every tab into individual tabs.
			for (var i = 0; i < op.text[0].length; i++) {
				var _pos = 0 + i;

				_.delay(handleLocalInput, i + 1, line, _pos, TAB);
			}

			return;
		} // Some weird CodeMirror shit
		else if (op.text[0] == EMPTY) {
			return;
		} // Is this every other case? 
		else {
			inputChar = op.text[0];
		}

		handleLocalInput(line, pos, inputChar);
	} else if (origin == DELETE_OP) {
		// TODO deal with block deletion, or at least find a way to avoid it

		handleLocalDelete(line, pos);
	} else if (origin.startsWith(REMOTE_INPUT_OP_PREFIX)) {
		const id = origin.substring(REMOTE_INPUT_OP_PREFIX.length);

		updateMapping(line, pos, id);
	}
}

/******************************* LOCAL HANDLERS *******************************/

function handleLocalInput(line, pos, val) {
	const id = getID();

	var prevElem, nextElem, prev, next;
	prevElem = getPrevElem(line, pos);
	if (prevElem !== undefined) {
		prev = prevElem.id;
		next = prevElem.next;

		prevElem.next = id;
	} else {
		next = mapping[line] !== undefined ? mapping[line][pos] : undefined;
	}

	if (next !== undefined) {
		nextElem = CRDT[next];

		nextElem.prev = id;
	}

	// Update CRDT
	const elem = new Element(id, prev, next, val, false);
	CRDT[id] = elem;

	updateMapping(line, pos, id);
}

function handleLocalDelete(line, pos) {
	if (mapping.length == 0) return;

	const id = mapping[line][pos];
	const elem = CRDT[id];

	if (elem === undefined) return;

	elem.del = true;
	if (mapping[line].length > 0) mapping[line].splice(pos, 1); 
	if (mapping[line].length == 0) mapping.splice(line, 1);

	if (debugMode) {
		console.log("Observed remove at line: " + line + " pos: " + pos);
	}
}

/******************************* REMOTE HANDLERS *******************************/

function handleRemoteOperation(id, prevId, type, val) {
	if (CRDT[id] !== undefined) return;

	if (type == INPUT_OP) handleRemoteInput(id, prevId, val);
	else if (type == DELETE_OP) handleDeleteOperations(id);
}

function handleRemoteInput(id, prevId, val) {
	var prevElem, nextElem, prev, next;
	prevElem = CRDT[prevId];
	if (prevElem !== undefined) {
		prev = prevElem.id;
		next = prevElem.next;

		prevElem.next = id;
	} else {
		// WHAT THE FUCK DO I DO?!?!??!
	}

	if (next !== undefined) {
		nextElem = CRDT[next];

		nextElem.prev = id;
	}

	// Update CRDT
	const elem = new Element(id, prev, next, val, false);
	CRDT[id] = elem;

	/***** Determine where to apply the operation *****/

	// Do a backward traversal until you find a non-deleted element.
	while (prevElem !== undefined && prevElem.del == true) prevElem = CRDT[prevElem.prev];

	// Find the element in the mapping.
	var line = 0;
	var pos = 0;
	if (prevElem !== undefined) {
		var stop = false;
		mapping.forEach(function(_line, i){
			_line.forEach(function(id, j){
				if (id == prevId) {
					stop = true;

					if (prevElem.val === RETURN) pos = 0;
					else pos = j + 1;

					return;
				}
			});

			if (stop) {
				if (prevElem.val === RETURN) line = i + 1;
				else line = i;

				return;
			}
		});
	}

	const _pos = {line: line, ch: pos};
    editor.getDoc().replaceRange(val, _pos, _pos, REMOTE_INPUT_OP_PREFIX + id);
}

/* =============================================================================
							SEQUENCE CRDT/ELEMENT
   =============================================================================*/

// Global var for the sequence CRDT local to this client.
CRDT = new Array();

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
}

/*
	Get the previous element based on a line and position.
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

	return CRDT[prev];
}

/*
	Creates UID based on the current time and userID.

	Its possible that the timestamp is not unique, so we 
	append a counter to the end of the ID.
	(Note: This will come in handle if we want to do block operations).
*/
function getID() {
	var id = userID + '_' + Date.now() + '_0';

	const elem = CRDT[id];
	if (elem !== undefined) {
		var i = 1;
		while (CRDT[id + '_' + i] !== undefined) {
			i++;
		}

		id = id + '_' + i;
	}

	return id;
}

/**
	Gets the first element of a sequence CRDT.
*/
function getStartElem(_CRDT = CRDT) {
	var start = null;

	const ids = Object.keys(_CRDT);
	$(ids).each(function(index, id){
		const elem = _CRDT[id];
		if (elem.prev == undefined) {
			start = elem;
			return;
		}
	});

	return start;
}

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

	const val = CRDT[id].val;
	const _line = mapping[line];
	const thisElem = CRDT[_line[pos]];

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


/* =============================================================================
									UTILITIES
   =============================================================================*/

/*
	Checks if the current sequence CRDT matches the editors value. 
*/
function verifyConsistent() {
	var snippet = editor.getValue();
	var _snippet = crdtToSnippet();

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
		const elem = CRDT[id];
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
			snippet = snippet + _CRDT[id].val;
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
		valArray[i] = CRDT[id].val;
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

/*
	Converts a CRDT to a string.
*/
function crdtToSnippet(_CRDT = CRDT) {
	var _mapping = crdtToMapping(_CRDT);
	var snippet = mappingToSnippet(_CRDT, _mapping);

	return snippet;
}

/**
	Converts the CRDT to a 2D mapping array.
*/
function crdtToMapping(_CRDT = CRDT) {
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

		curElem = _CRDT[curElem.next];
	}

	return _mapping;
}
