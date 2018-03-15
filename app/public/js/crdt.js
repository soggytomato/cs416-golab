/*
	Object definition for a CRDT element.
*/
class Element {
    constructor(id, prev, next, val) {
    	this.id = id;
        this.prev = prev;
        this.next = next;
        this.val = val;
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

// Globals for CRDT sequence and mappings.
crdt = new Array();
crdtMappings = [];

$(document).ready(function(){
	// Handles all user inputs before they are applied to the editor.
	editor.on('beforeChange', function(cm, change){
		handleChange(change);
	});
});

/*
	Dispatches input or delete to 'handleInput' and 'handleRemove'.

	Note: this is very unrefined at the moment, it assumes that the text
		  entered/removed is always no more than one character. 
*/
function handleChange(change) {
	const line = change.from.line;
	const pos = change.from.ch;

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
	// Create UID based on the current time and userID.
	const id = userID + '_' + Date.now();

	// Add new line to mappings if necessary.
	if (crdtMappings[line] === undefined) {
		crdtMappings.push([]);
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

		crdtMappings[line].splice(pos, 0, id);
	}

	const elem = new Element(id, prev, next, val);

	crdt[id] = elem;
	crdtMappings[line][pos] = id;

	console.log("Observed input at line: " + line + " pos: " + pos + " char: " + val);
}

/**
	TODO
*/
function handleRemove(line, pos, val) {
	console.log("Observed remove at line: " + line + " pos: " + pos + " char: " + val);
}

/**
	Get the previous element based on te insertion at the line and pos.
*/
function getPrevElem(line, pos) {
	var prev = undefined;
	var prevElem = undefined;

	if (line == 0 && pos == 0) {
		prevElem = undefined;
	} else if (pos > 0) {
		prev = crdtMappings[line][pos - 1];
		prevElem = crdt[prev];
	} else if (line > 0) {
		prevLine = editor.getLineTokens(line-1);
		if (prevLine.length > 0) {
			prev = crdtMappings[line-1][prevLine[0].end];
			prevElem = crdt[prev];
		} else {
			prev = crdtMappings[line-1][0];
			prevElem = crdt[prev] ;
		}
	}

	return prevElem;
}

/**
	Get the next element based on te insertion at the line and pos.
*/
function getNextElem(line, pos) {
	const next = crdtMappings[line][pos];
	const nextElem = crdt[next];
	
	return nextElem;
}