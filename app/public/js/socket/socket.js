socket = undefined;

HANDLE_OP_CMD = 'HandleOp';

function initWS(workerIP) {
    console.log("Trying to connect to: " + "ws://" + workerIP + "/ws");

    socket = new WebSocket("ws://" + workerIP + "/ws?userID=" + userID);
    statusHTML = $('#status');

    socket.onopen = onOpen;
    socket.onclose = onClose;
    socket.onmessage = onMessage;
}

function onOpen() {
    const msg = { 
        SessionID: sessionID, 
        Username: userID, 
        Command: "GetSessCRDT"
    };

    socket.send(JSON.stringify(msg));
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

function sendInput(id, prevId, val) {
    const msg = {
        SessionID: sessionID,
        Username: userID,
        Command: HANDLE_OP_CMD,
        Type: INPUT_OP,
        ID: id,
        PrevID: prevId,
        Val: val
    };

    socket.send(JSON.stringify(msg));
}

function sendDelete(id) {
    const msg = {
        SessionID: sessionID,
        Username: userID,
        Command: HANDLE_OP_CMD,
        Type: DELETE_OP,
        ID: id
    };

    socket.send(JSON.stringify(msg));
}