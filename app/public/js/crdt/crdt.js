/* =============================================================================
							ELEMENT CLASS DEFINITION
   =============================================================================*/
/*
	An Element is simply an entry in the sequence CRDT.
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

/* =============================================================================
							CRDT CLASS DEFINITION
   =============================================================================*/
/*
	A SeqCRDT is a local list of all elements that comprise a snippet.
*/
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
		var snippet = mapping.toSnippet();

		return snippet;
	}

	/*
	Converts the CRDT to a mapping.*/
	toMapping() {
		var _mapping = new Mapping();

		var curElem = this.getFirstElement();
		var lastVal, lastPos, lastLine;
		while (curElem != undefined) {
			if (curElem.del !== true) {
				var curLine, curPos;
				if (lastLine == undefined) {
					_mapping.addLine();
					curLine = 0;
				} else if (lastVal == RETURN) {
					_mapping.addLine();
					curLine = lastLine + 1;
				}

				if (lastPos == undefined || lastVal == RETURN) { 
					curPos = 0;
				} else {
					curPos = lastPos + 1;
				}

				_mapping.set(curLine, curPos, curElem.id);

				lastVal = curElem.val;
				lastPos = curPos;
				lastLine = curLine;
			}

			curElem = this.get(curElem.next);
		}

		return _mapping;
	}

	/*
	Checks if the current sequence CRDT matches the editors value. */
	verify() {
		var snippet = editor.getValue();
		var _snippet = this.toSnippet();

		if (snippet != _snippet) {
			alert('CRDT and editor fell out of sync! Check console for details.');
			console.error('Snippet is not consistent!\n' + 'In editor: \n' + snippet + '\nFrom CRDT:\n' + _snippet);

			return false;
		} else {
			return true;
		}
	}
}
CRDT = new SeqCRDT();