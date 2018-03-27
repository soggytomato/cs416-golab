// Globals
debugMode = false;
userID = "";
sessionID = "";
currentSessions = [];
currentWorkerIP = ""
jobIDs = []
workerIP = '';

$(document).ready(function() {
    $('.input-wrapper').resizable({
        handles: 's',
        resize: function() {
            var curH = $(this).outerHeight();
            var curW = $(this).width();
            var containerH = $('.left').outerHeight();

            $('.input').height(curH - 30);
            editor.setSize(curW, curH - 30);

            $('.output-wrapper').height(containerH - curH);
            $('.output').height(containerH - curH - $('.output-wrapper').find('span').height());
        }
    });
});

$(document).ready(function() {
    $.ajax({
        type: 'get',
        url: '/sessions',
        success: function(data) {
            var select = document.getElementById("sessionSelect");
            for (var i = 0; i < data.ExistingSessions.length; i++) {
                var opt = data.ExistingSessions[i];
                var el = document.createElement("option");
                el.textContent = opt;
                el.value = opt;
                select.appendChild(el);
            }
            currentSessions = data.ExistingSessions
        }
    })
    $.ajax({
        type: 'get',
        url: '/usernames',
        success: function(data) {
            var select = document.getElementById("userSelect");
            for (var i = 0; i < data.AllUsernames.length; i++) {
                var opt = data.AllUsernames[i];
                var el = document.createElement("option");
                el.textContent = opt;
                el.value = opt;
                select.appendChild(el);
            }
        }
    })
    formBindings();
});

function reset() {
    editor.setValue(CRDT.toSnippet());
    editor.setOption("readOnly", false);
    document.getElementById('outputBox').innerHTML = "";
    document.getElementById("snipTitle").style.color = ''
    document.getElementById('snipTitle').innerHTML = "Snippet:"
}

function execute() {
    var newForm = document.createElement('form');
    newForm.setAttribute('id', 'executeForm');
    newForm.setAttribute('form', 'executeForm');

    var sessInput = document.createElement('input');
    sessInput.setAttribute('name', 'sessionID');
    sessInput.setAttribute('value', sessionID);
    sessInput.setAttribute('type', 'hidden');

    var snippet = document.createElement('textarea');
    snippet.setAttribute('name', 'snippet');
    snippet.value = CRDT.toSnippet();
    snippet.setAttribute('class', 'text');
    snippet.setAttribute('form', 'executeForm');

    newForm.append(sessInput);
    newForm.append(snippet);
    console.log(snippet)
    $("body").append(newForm);
    console.log($('#executeForm').serialize());

    $.ajax({
        type: 'post',
        url: "http://" + currentWorkerIP + '/execute',
        dataType: 'json',
        data: $('#executeForm').serialize(),
        success: function(data) {
            jobIDs.push(data.JobID);
            console.log(jobIDs)
            $("#logList").prepend("<li><a href=# id=" + data.JobID + ">" + data.JobID + "</a></li>")
        }
    })
    newForm.parentNode.removeChild(newForm);
}

function formBindings() {
    $('#newUserRadio').on('click', function() {
        $('.new-user-group').css('display', 'block');
        $('.select-user-group').css('display', 'none');
    });

    $('#existingUserRadio').on('click', function() {
        $('.new-user-group').css('display', 'none');
        $('.select-user-group').css('display', 'block');
    });

    $('#newSessionRadio').on('click', function() {
        $('.new-session-group').css('display', 'block');
        $('.select-session-group').css('display', 'none');
    });

    $('#existingSessionRadio').on('click', function() {
        $('.new-session-group').css('display', 'none');
        $('.select-session-group').css('display', 'block');
    });

    $('#register').submit(function(e) {
        e.preventDefault();

        var valid = verifyRegister();

        if (valid) {
            $.ajax({
                type: 'post',
                url: '/register',
                dataType: 'json',
                data: $('#register').serialize(),
                success: function(data) {
                    if (data.WorkerIP.length == 0) {
                        alert("No available Workers, please try again later")
                    } else {
                        workerIP = data.WorkerIP;
                        initWS()
                        $('.register').css('display', 'none');
                        $('.editor').slideDown('slow');
                        currentWorkerIP = data.WorkerIP
                        setTimeout(function() {
                            editor.refresh();
                        }, 500);
                    }
                }
            })
        }

        return false;
    });
};

function verifyRegister() {
    var valid = true;

    if ($('#existingUserRadio').is(':checked')) {
        if ($('#userSelect').find(':selected').attr('placeholder') === "") {
            alert("Please pick a user name.");
            valid = false;
        } else {
            userID = $('#userSelect').find(':selected').text();
        }
    } else {
        if ($('#userInput').val() == "") {
            alert("Please enter a user name.");
            valid = false;
        } else if ($('#userInput').val().indexOf(' ') >= 0) {
            alert("User name cannot contain whitespace.");
            valid = false;
        } else {
            userID = $('#userInput').val();
        }
    }

    if (valid == false) {
        return
    }

    if ($('#existingSessionRadio').is(':checked')) {
        if ($('#sessionSelect').find(':selected').attr('placeholder') === "") {
            alert("Please pick a session name.");
            valid = false;
        } else {
            sessionID = $('#sessionSelect').find(':selected').text();
        }
    } else {
        if ($('#sessionInput').val() == "") {
            alert("Please enter a session ID.");
            valid = false;
        } else if ($('#sessionInput').val().indexOf(' ') >= 0) {
            alert("Session ID cannot contain whitespace.");
            valid = false;
        } else {
            sessionID = $('#sessionInput').val();
            if (currentSessions.includes(sessionID)) {
                alert("Session ID is already taken, please enter a unique Session ID")
                valid = false
            }
        }
    }

    return valid;
}