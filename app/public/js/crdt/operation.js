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
					CRDT.verify();
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

		mapping.update(line, pos, id);
	}
}

/******************************* LOCAL HANDLERS *******************************/

function handleLocalInput(line, pos, val) {
	const id = CRDT.getNewID();

	var prevElem, nextElem, prev, next;
	prevElem = CRDT.get(mapping.getPreceding(line, pos));
	if (prevElem !== undefined) {
		prev = prevElem.id;
		next = prevElem.next;

		prevElem.next = id;
	} else {
		next = mapping.getLine(line) !== undefined ? mapping.get(line, pos) : undefined;
	}

	if (next !== undefined) {
		nextElem = CRDT.get(next);

		nextElem.prev = id;
	}

	// Update CRDT
	CRDT.set(id, new Element(id, prev, next, val, false));

	mapping.update(line, pos, id);
}

function handleLocalDelete(line, pos) {
	if (mapping.length() == 0) return;

	const id = mapping.get(line, pos);
	const elem = CRDT.get(id);

	if (elem === undefined) return;

	elem.del = true;
	if (mapping.lineLength(line) > 0) mapping.delete(line, pos); 
	if (mapping.lineLength(line) == 0) mapping.deleteLine(line);

	if (debugMode) {
		console.log("Observed remove at line: " + line + " pos: " + pos);
	}
}

/******************************* REMOTE HANDLERS *******************************/

function handleRemoteOperation(id, prevId, type, val) {
	if (CRDT.get(id) !== undefined) return;

	if (type == INPUT_OP) handleRemoteInput(id, prevId, val);
	else if (type == DELETE_OP) handleDeleteOperations(id);
}

function handleRemoteInput(id, prevId, val) {
	var prevElem, nextElem, prev, next;
	prevElem = CRDT.get(prevId);
	if (prevElem !== undefined) {
		prev = prevElem.id;
		next = prevElem.next;

		prevElem.next = id;
	} else {
		// WHAT THE FUCK DO I DO?!?!??!
	}

	if (next !== undefined) {
		nextElem = CRDT.get(next);

		nextElem.prev = id;
	}

	// Update CRDT
	CRDT.set(id, new Element(id, prev, next, val, false));

	// Do a backward traversal until a non-deleted element is found.
	while (prevElem !== undefined && prevElem.del == true) prevElem = CRDT.get(prevElem.prev);

	// Find the element in the mapping.
	var line = 0;
	var pos = 0;
	if (prevElem !== undefined) {
		var stop = false;
		mapping.getLines().forEach(function(_line, i){
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

	// Set the value in the editor at the line and position.
	const _pos = {line: line, ch: pos};
    editor.getDoc().replaceRange(val, _pos, _pos, REMOTE_INPUT_OP_PREFIX + id);
}
