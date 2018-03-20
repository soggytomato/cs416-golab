socket = undefined;

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

function onMessage(msg) {
    // Handle the op
}

function sendInput(id, prevId, val) {
    const msg = {
        SessionID: sessionID,
        Username: userID,
        Command: 'HandleOp',
        Payload: {
            Type: 'input',
            ID: id,
            PrevId: prevId,
            Val: val
        }
    };

    socket.send(JSON.stringify(msg));
}

function sendDelete(id) {
    const msg = {
        SessionID: sessionID,
        Username: userID,
        Command: 'HandleOp',
        Payload: {
            Type: 'delete',
            ID: id
        }
    };

    socket.send(JSON.stringify(msg));
}