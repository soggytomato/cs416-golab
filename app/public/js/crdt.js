crdt 	= new Array();
mapping = [];

$(document).ready(function(){
	editor = CodeMirror.fromTextArea(document.getElementById("code"), {
		theme: "dracula",
		matchBrackets: true,
		indentUnit: 0,
		tabSize: 4,
		indentWithTabs: false,
		electricChars: false,
		mode: "text/x-go"
	});


	// Handles all user inputs before they are applied to the editor.
	editor.on('beforeChange', 
		function(cm, change){
			handleChange(change);
		}
	);

	if (debugMode) {
		// Verifies snippet after processing handle.
		editor.on('change', function(cm, change){
			verifyConsistent();
		});
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
	const line = change.from.line;
	const pos = getPos(line, change.from.ch);

	if (change.origin == "+input") {
		var inputChar;
		if (change.text.length == 2 && change.text[0] == '' && change.text[1] == '') {
			inputChar = '\n';
		} else {
			inputChar = change.text[0];
		}

		handleInput(line, pos, inputChar);
	} else if (change.origin == '+delete') {
		// Temporary hack so that block deletion doesnt complain
		if (change.removed) {
			handleRemove(line, pos, change.removed[0]);
		}
	}
}

/**
	Handles input events.
*/
function handleInput(line, pos, val) {
	var newLine = false;

	// Create UID based on the current time and userID.
	const id = userID + '_' + Date.now();

	// Add new line to mapping if necessary.
	if (mapping[line] === undefined) {
		mapping.push([]);
	} else {
		// Check if we are adding another indent.
		const elem = mapping[line][pos];
		const thisElem = crdt[elem];

		if (thisElem !== undefined && thisElem.val == '\n' && thisElem.val == val) {
			newLine = true;

			line = line + 1;
			pos = 0;

			mapping.splice(line, 0, []);
		}
	}

	// Get the previous element and set this to its next element;
	const prevElem = getPrevElem(line, pos);
	const prev = prevElem ? prevElem.id : undefined;
	if (prevElem != undefined) {
		prevElem.setNext(id);
	}

	// Get next element and set this to its previous element,
	//	then shift the mappings to add this element.
	const nextElem = getNextElem(line, pos);
	const next = nextElem ? nextElem.id : undefined;
	if (nextElem != undefined) {
		nextElem.setPrev(id);

		if (newLine == false) {
			mapping[line].splice(pos, 0, id);
		}
	}

	const elem = new Element(id, prev, next, val);

	crdt[id] = elem;
	mapping[line][pos] = id;

	if (debugMode) {
		console.log("Observed input at line: " + line + " pos: " + pos + " char: " + unescape(val));		
	}
}

/**
	TODO
*/
function handleRemove(line, pos, val) {
	if (debugMode) {
		console.log("Observed remove at line: " + line + " pos: " + pos + " char: " + unescape(val));
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
*/
function getNextElem(line, pos) {
	var next = undefined;

	const _line = mapping[line];
	if (_line.length > 0) {
		next = _line[pos];
	} else {
		if (mapping[line+1] !== undefined) {
			next = mapping[line+1][0];
		}
	}
	
	return crdt[next];
}

function crdtToMapping(_crdt) {
	var _mapping = [];

	var curElem = getStartElemID();
	var lastVal, lastPos, lastLine;
	while (curElem != undefined) {
		var curLine, curPos;
		if (lastLine == undefined) {
			_mapping.push([]);
			curLine = 0;
		} else if (lastVal == '\n') {
			_mapping.push([]);
			curLine = lastLine + 1;
		}

		if (lastPos == undefined || lastVal == '\n') { 
			curPos = 0;
		} else {
			curPos = lastPos + 1;
		}

		_mapping[curLine][curPos] = curElem.id;

		lastVal = curElem.val;
		lastPos = curPos;
		lastLine = curLine;

		curElem = _crdt[curElem.next];
	}

	return _mapping;
}

function mappingToSnippet(_mapping, _crdt) {
	var snippet = "";

	_mapping.forEach(function(line){
		line.forEach(function(id){
			snippet = snippet + _crdt[id].val;
		});
	});

	return snippet;
}

function crdtToSnippet(_crdt) {
	var mapping = crdtToMapping(_crdt);
	var snippet = mappingToSnippet(mapping, _crdt);

	return snippet;
}

function getStartElemID() {
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

function verifyConsistent() {
	var snippet = editor.getValue();
	var _snippet = crdtToSnippet(crdt);

	if (snippet != _snippet) {
		console.error('Snippet is not consistent!\n' + 'In editor: \n' + snippet + '\nFrom CRDT:\n' + _snippet);
	}
}

function getPos(line, ch) {
	var pos = 0;

	const _line = mapping[line];
	if (_line !== undefined && _line.length > 0) {
		_line.forEach(function(id){
			const val = crdt[id].val;

			if (val != '\n') pos++;
			if (val.length >= ch) return;
		});
	}

	return pos;
}





