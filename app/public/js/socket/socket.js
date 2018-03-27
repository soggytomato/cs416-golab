socket = undefined;
$(window).on('beforeunload', function(event) {
    $.ajax({ type: 'post', url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID });
});

function initWS() {
    socket = new WebSocket("ws://" + workerIP + "/ws?userID=" + userID + '&sessionID=' + sessionID);
    statusHTML = $('#status');

    socket.onopen = onOpen;
    socket.onclose = onClose;
    socket.onmessage = onMessage;
}

function onOpen() {
    initCRDT();
}

function onClose() {
    $.ajax({ type: 'post', url: 'http://' + workerIP + '/session?userID=' + userID + '&sessionID=' + sessionID });
}

function onMessage(_msg) {
    const element = JSON.parse(_msg.data);
    if (element.hasOwnProperty('Job')) {
        matchLog(element);
    } else {
        handleRemoteOperation(element);
    }
}

function sendElement(id) {
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