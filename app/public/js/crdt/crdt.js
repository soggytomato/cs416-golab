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
        var _mapping = this.toMapping();
        var snippet = _mapping.toSnippet();

        return snippet;
    }

    /*
    Converts the CRDT to a mapping.*/
    toMapping() {
        var _mapping = new Mapping();

        var curElem = this.head;
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
CRDT = new SeqCRDT();

function initCRDT() {
    $.ajax({
        type: 'get',
        url: 'http://' + workerIP + '/session',
        data: {
            sessionID: sessionID
        },
        success: function(data) {
            // CRDT Init
            const crdt = data.SessionRecord.CRDT;

            const ids = Object.keys(data.SessionRecord.CRDT);
            ids.forEach(function(id) {
                const element = crdt[id];
                const prev = element.PrevID == "" ? undefined : element.PrevID;
                const next = element.NextID == "" ? undefined : element.NextID;
                const val = element.Text;
                const del = element.Deleted;

                CRDT.seq[id] = new Element(id, prev, next, val, del)
            });

            CRDT.head = CRDT.getHead();
            CRDT.length = ids.length;

            mapping = CRDT.toMapping();

            editor.setValue(CRDT.toSnippet());

            // Log Records Init 
            const logs = data.LogRecord
            if (logs != null) {
                for (var i = 0; i < logs.length; i++) {
                    jobIDs.set(logs[i].Job.JobID, logs[i].Job.Done);
                    $("#logList").prepend("<li><a href=# id=" + logs[i].Job.JobID + ">" + logs[i].Job.JobID + "</a></li>")
                    if (logs[i].Job.Done) {
                        var logOutput = document.getElementById(logs[i].Job.JobID);
                        var _log = logs[i];
                        (function(_log) {
                            logOutput.addEventListener('click', function(e) {
                                e.preventDefault();
                                logClicked(_log);
                            }, false);
                        })(_log);
                    }
                }
            }
        }
    })
}