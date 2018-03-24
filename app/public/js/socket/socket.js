socket = undefined;

function initWS() {
    socket = new WebSocket("ws://" + workerIP + "/ws?userID=" + userID + '&sessionID='+sessionID);
    statusHTML = $('#status');

    socket.onopen = onOpen;
    socket.onclose = onClose;
    socket.onmessage = onMessage;
}

function onOpen() {
    // GET THE SESS CRDT THROUGH A GET
}

function onClose() {
    console.log("Socket Close")
    // TODO, connect to new worker if possible
}

function onMessage(_msg) {
    const msg = JSON.parse(_msg.data);

    const cmd = msg.Command;
    if (cmd == HANDLE_OP_CMD) {
        handleRemoteOperation(msg);
    }
}

function sendElement(id) {
    const _element = CRDT.get(id);
    const element = {
        SessionID: sessionID,
        ClientID: userID,
        ID: _element.id.toString(),
        PrevID: _element.prevId,
        Text: _element.val,
        Deleted: _element.del
    };

    socket.send(JSON.stringify(element));
}
