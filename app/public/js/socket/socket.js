socket = undefined;
unload = false;
$(window).on('beforeunload', function(event) {
    closeSession();
});

function initWS() {
    socket = new WebSocket("ws://" + workerIP + "/ws?userID=" + userID + '&sessionID=' + sessionID);
    statusHTML = $('#status');

    socket.onopen = onOpen;
    socket.onclose = onClose;
    socket.onmessage = onMessage;
    socket.onerror = onError;
}

function onOpen() {
    initCRDT();
}

function onClose(e) {
    console.log("CLOSE: " + e);

    if (!unload) {
        closeSession();
        recover();
    }
}

function onError(e) {
    console.log("ERROR: " + e);

    if (!unload) {
        closeSession();
        recover();
    }
}

function onMessage(_msg) {
    const element = JSON.parse(_msg.data);
    if (element.hasOwnProperty('Job')) {
        matchLog(element);
    } else {
        handleRemoteOperation(element);
    }
}

function sendElement(_element) {
    const element = {
        SessionID: sessionID,
        ClientID: userID,
        ID: _element.id,
        PrevID: _element.prev,
        Text: _element.val,
        Deleted: _element.del
    };

    socket.send(JSON.stringify(element));
}

function sendElementByID(id) {
    const _element = CRDT.get(id);
    const element = {
        SessionID: sessionID,
        ClientID: userID,
        ID: _element.id,
        PrevID: _element.prev,
        Text: _element.val,
        Deleted: _element.del
    };

    socket.send(JSON.stringify(element));
}

function closeSession() {
    unload = true;

    $.ajax({
        type: 'post',
        url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID
    });

    if (socket.readyState == 0 || socket.readyState == 1) socket.close();
}