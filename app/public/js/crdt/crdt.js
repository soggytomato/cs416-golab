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
	constructor(seqCRDT = new Array(), first) {
    	this.seq = seqCRDT;
    	this.first = first;
    	this.length = Object.keys(seqCRDT).length;
    }

    get(id) {
    	return this.seq[id];
    }

    set(id, elem) {
    	this.seq[id] = elem;

  		this.length++;
    }

    length() {
    	return this.seq.length;
    }

    /*
    Creates UID based on increment. */
	getNewID() {
		return this.length + "_" + userID;
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

			logOpsString();

			return false;
		} else {
			return true;
		}
	}
}
CRDT = undefined;

function initCRDT() {
  $.ajax({
      type: 'get',
      url: 'http://'+workerIP+'/getSession',
      data: {sessionID: sessionID},
      success: function(data) {
        CRDT = new SeqCRDT(data.CRDT, data.Head)
      }
  })
}
