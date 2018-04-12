outOfSync = false;

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
    constructor(seqCRDT = new Array(), head) {
        this.seq = seqCRDT;
        this.head = head;
        this.length = Object.keys(seqCRDT).length;
    }

    get(id) {
        return this.seq[id];
    }

    set(id, elem) {
        this.seq[id] = elem;
        this.length++;

        if (elem.prev == undefined) this.head = elem;
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
    Gets the head element of this CRDT.*/
    getHead() {
        const _this = this;

        var start = null;

        const ids = Object.keys(this.seq);
        $(ids).each(function(index, id) {
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
        const _mapping = this.toMapping();
        if (_mapping != undefined) {
            return _mapping.toSnippet();
        } else {
            return undefined;
        }
    }

    /*
    Converts the CRDT to a mapping.*/
    toMapping() {
        var fail = false;
        const start = Date.now();

        var _mapping = new Mapping();

        var curElem = this.head;
        var lastVal, lastPos, lastLine;
        while (curElem != undefined) {
            if (Date.now() - start > 10000) {
                if (debugMode) showError('Failed to verify CRDT.');
                fail = true;

                break;
            }

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

        if (fail) {
            return undefined;
        } else {
            return _mapping;
        }
    }

    /*
    Checks if the current sequence CRDT matches the editors value. */
    verify() {
        var snippet = editor.getValue();
        var _snippet = this.toSnippet();

        if (_snippet == undefined) return;

        if (snippet != _snippet) {
            if (!outOfSync) {
                if (debugMode) showError('CRDT and editor fell out of sync! Check console for details.', 6000);
                outOfSync = true;
            }

            console.error('Snippet is not consistent!\n' + 'In editor: \n' + snippet + '\nFrom CRDT:\n' + _snippet);
            logOpsString();

            return false;
        } else {
            if (outOfSync) {
                if (debugMode) showSuccess('CRDT and editor back in sync!');
                outOfSync = false;
            }

            return true;
        }
    }
}
CRDT = new SeqCRDT();