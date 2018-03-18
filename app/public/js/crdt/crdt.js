/* =============================================================================
							SEQUENCE CRDT/ELEMENT
   =============================================================================*/

class SeqCRDT {
	constructor(seqCRDT = new Array()) {
    	this.seq = seqCRDT;
    }

    get(id) {
    	return this.seq[id];
    }

    set(id, elem) {
    	this.seq[id] = elem;
    }

    /* 
    Creates UID based on the current time and userID.

	Its possible that the timestamp is not unique, so we append a counter to the end of the ID.
	(Note: This will come in handle if we want to do block operations).	*/
	getNewID() {
		var id = userID + '_' + Date.now() + '_0';

		const elem = this.seq[id];
		if (elem !== undefined) {
			var i = 1;
			while (this.seq[id + '_' + i] !== undefined) {
				i++;
			}

			id = id + '_' + i;
		}

		return id;
	}

	/*
	Gets the first element of this CRDT.*/
	getFirstElement() {
		const _this = this;

		var start = null;

		const ids = Object.keys(this.seq);
		$(ids).each(function(index, id){
			const elem = _this.seq[id];
			if (elem.prev == undefined) {
				start = elem;
				return;
			}
		});

		return start;
	}

	/*
	Converts CRDT to a snippet string.*/
	toSnippet() {
		var _mapping = this.toMapping();
		var snippet = mappingToSnippet(this, _mapping);

		return snippet;
	}

	/*
	Converts the CRDT to a mapping.*/
	toMapping() {
		var _mapping = [];

		var curElem = this.getFirstElement();
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

			curElem = this.seq[curElem.next];
		}

		return _mapping;
	}
}
CRDT = new SeqCRDT();

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

	return CRDT.get(prev);
}