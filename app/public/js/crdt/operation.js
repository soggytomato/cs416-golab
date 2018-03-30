// String constants
RETURN = '\n';
TAB = '\t';
SPACE = ' ';
EMPTY = '';

// Operation constants
INPUT_OP = '+input';
DELETE_OP = '+delete';
REMOTE_INPUT_OP = '+remote_input';
REMOTE_DELETE_OP = '+remote_delete';
REMOTE_INPUT_OP_PREFIX = REMOTE_INPUT_OP + '_';
REMOTE_DELETE_OP_PREFIX = REMOTE_DELETE_OP + '_';

// Queue of changes
changes = new Array();

// Flages whether the changes are currently being addressed
changesInProgress = false

/*
    Register the handlers for events coming from CodeMirror.

    Everytime there is an operation (key stroke), it will first hit the
    'beforeChange' event handler which will process the operation before
    the key stroke is actually applied to the text area that the user sees.

    The 'change' event occurs after the operation has been processed and
    added to the text area. At this point, we check consistency
    between the editors text area and the CRDT, given we are in debug mode.
*/
$(document).ready(function() {
    editor = CodeMirror.fromTextArea(document.getElementById("code"), {
        theme: "dracula",
        matchBrackets: true,
        indentUnit: 4,
        tabSize: 4,
        indentWithTabs: true,
        electricChars: true,
        smartIndent: false,
        mode: "text/x-go",
        lineNumbers: true
    });

    // Handles all user inputs before they are applied to the editor.
    editor.on('beforeChange',
        function(cm, change) {
            if (origin == DELETE_OP && change.from.hitSide) return;

            // Push to queue of changes and init a new Promise
            changes.push(change);
            initOpPromise();
        }
    );
});

// Starts a new Promise if changes are not being handled already
function initOpPromise() {
    if (changesInProgress) return;

    changesInProgress = true;

    var promise = new Promise(processOpPromise);
    promise.then(initOpPromise, endOpPromise);
}

// Kicks of if the last Promise saw the end of changes
function endOpPromise() {
    // It's possible that between kicking this off, a new
    // op has come in. If so, kick off a new Promise.
    if (changes.length > 0) {
        initOpPromise();
        return;
    }

    // Otherwise wrap things up.
    changesInProgress = false;

    if (debugMode) CRDT.verify();
}

// Processes the actual operation
function processOpPromise(promise, promiseEnd) {
    handleOperation(changes[0]);
    
    changes.splice(0, 1);
    if (changes.length > 0) {
        promise();
    } else {
        promiseEnd();
    }
}

/*
    Dispatches input or delete to 'handleInput' and 'handleRemove'.

    Note: this is very unrefined at the moment, it assumes that the text
          entered/removed is always no more than one 'character'.
*/
function handleOperation(op) {
    if (debugMode) ops.push(op);

    var line = op.from.line;
    var ch = op.from.ch;

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
                var _ch = 0 + i;

                _.delay(handleLocalInput, i + 1, line, _ch, TAB);
            }

            return;
        } // Some weird CodeMirror shit
        else if (op.text[0] == EMPTY) {
            return;
        } // Is this every other case?
        else {
            inputChar = op.text[0];
        }

        handleLocalInput(line, ch, inputChar);
    } else if (origin == DELETE_OP) {
        // TODO deal with block deletion, or at least find a way to avoid it

        handleLocalDelete(line, ch);
    } else if (origin.startsWith(REMOTE_INPUT_OP_PREFIX)) {
        const id = origin.substring(REMOTE_INPUT_OP_PREFIX.length);

        mapping.update(line, ch, id);
    } else if (origin.startsWith(REMOTE_DELETE_OP_PREFIX)) {
        mapping.delete(line, ch);
    }
}

/******************************* LOCAL HANDLERS *******************************/

// Cache of local elements that haven't been ACK'd yet
cache = []

function handleLocalInput(line, ch, val) {
    const id = CRDT.getNewID();

    var prevElem, nextElem, prev, next;
    prevElem = CRDT.get(mapping.getPreceding(line, ch));
    if (prevElem !== undefined) {
        prev = prevElem.id;
        next = prevElem.next;

        prevElem.next = id;
    } else {
        next = mapping.getLine(line) !== undefined ? mapping.get(line, ch) : undefined;
    }

    if (next !== undefined) {
        nextElem = CRDT.get(next);

        nextElem.prev = id;
    }

    // Update CRDT and mapping
    const elem = new Element(id, prev, next, val, false);
    CRDT.set(id, elem);
    mapping.update(line, ch, id);

    // Push to the cache
    cache.push(elem);

    sendElementByID(id);

    if (debugMode) console.log("Observed input at line: " + line + " pos: " + ch + " char: " + unescape(val));
}

function handleLocalDelete(line, ch) {
    if (mapping.length() == 0) return;

    const id = mapping.get(line, ch);
    const elem = CRDT.get(id);

    if (elem === undefined) return;
    else elem.del = true;

    // Push to the cache
    cache.push(elem);

    // Apply to the editor
    mapping.delete(line, ch);

    sendElementByID(id);

    if (debugMode) console.log("Observed remove at line: " + line + " pos: " + ch);
}

/******************************* REMOTE HANDLERS *******************************/

function handleRemoteOperation(op) {
    const id = op.ID;
    const prevId = op.PrevID == "" ? undefined : op.PrevID;
    const val = op.Text;
    const del = op.Deleted;

    // Cycle through cache
    var index = undefined;
    cache.forEach(function(elem, i) {
        if (elem.id == id && elem.del == elem.del) {
            index = i;
            return false;
        }
    });

    if (index !== undefined) {
        cache.splice(index, 1);
    }

    if (del == false) handleRemoteInput(id, prevId, val);
    else handleRemoteDelete(id);
}

function handleRemoteInput(id, prevId, val) {
    if (CRDT.get(id) !== undefined) return;

    var prevElem, nextElem, prev, next;

    prevElem = CRDT.get(prevId);
    while (prevElem !== undefined) {
        next = prevElem.next;

        if (next === undefined || next < id) {
            break;
        } else {
            prevElem = CRDT.get(next);
        }
    }

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
    var ch = 0;
    if (prevElem !== undefined) {
        var stop = false;

        const lines = mapping.getLines();
        for (var i = 0; i < lines.length; i++) {
            const _line = lines[i];

            _line.forEach(function(id, j) {
                if (id == prevElem.id) {
                    stop = true;

                    if (prevElem.val === RETURN) ch = 0;
                    else ch = j + 1;

                    return;
                }
            });

            if (stop) {
                if (prevElem.val === RETURN) line = i + 1;
                else line = i;

                break;
            }
        }
    }

    // Apply to the editor
    const pos = {
        line: line,
        ch: ch
    };
    editor.getDoc().replaceRange(val, pos, pos, REMOTE_INPUT_OP_PREFIX + id);

    if (debugMode) console.log("Observed input at line: " + line + " pos: " + ch + " char: " + unescape(val));
}

function handleRemoteDelete(id) {
    const elem = CRDT.get(id);

    // TODO: if undefined, deal with it
    if (elem === undefined || elem.del == true) return;
    else elem.del = true;

    // Apply to the editor
    const pos1 = mapping.getPosition(id);
    var pos2 = {
        line: undefined,
        ch: undefined
    };
    if (elem.val == RETURN) {
        pos2.line = pos1.line + 1;
        pos2.ch = 0;
    } else {
        pos2.line = pos1.line;
        pos2.ch = pos1.ch + 1;
    }

    if (pos1.line != undefined && pos1.ch !== undefined) editor.getDoc().replaceRange('', pos1, pos2, REMOTE_DELETE_OP_PREFIX + id);

    if (debugMode) console.log("Observed remove at line: " + pos1.line + " pos: " + pos1.ch);
}

/******************************* UTILITY *******************************/

// Array of all changes -- only for debug purposing to replay until
// a sync problem is observed
ops = []

function replayOperations(ops, rate = 500) {
    if (typeof ops == "string") {
        ops = getOpsFromString(ops);
    }

    const keys = Object.keys(ops);
    var i = 0;

    var interval = setInterval(function() {
        if (keys[i] == undefined) {
            clearInterval(interval);
            return;
        }

        const key = keys[i];
        const op = ops[key];
        const origin = op.origin;
        if (origin == INPUT_OP || origin.startsWith(REMOTE_INPUT_OP_PREFIX)) {
            editor.getDoc().replaceRange(op.text, op.from, op.to, INPUT_OP);
        } else if (origin == DELETE_OP || origin.startsWith(REMOTE_DELETE_OP_PREFIX)) {
            editor.getDoc().replaceRange(op.text, op.from, op.to, DELETE_OP);
        }

        i++;
    }, rate);
}

function logOpsString() {
    const opsString = "'" + JSON.stringify(ops) + "'";
    console.log("Operation string: \n" + opsString);
}

function getOpsFromString(opsString) {
    return JSON.parse(opsString);
}