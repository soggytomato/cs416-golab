// Globals
userID = "";
sessionID = "";

$(document).ready(function(){
	editor = CodeMirror.fromTextArea(document.getElementById("code"), {
        theme: "dracula",
        matchBrackets: true,
        indentUnit: 8,
        tabSize: 8,
        indentWithTabs: true,
        mode: "text/x-go"
      });

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

$(document).ready(function(){
	formBindings();
});

function formBindings() {
	$('#newUserRadio').on('click',function(){
		$('.new-user-group').css('display', 'block');
		$('.select-user-group').css('display', 'none');
	});

	$('#existingUserRadio').on('click',function(){
		$('.new-user-group').css('display', 'none');
		$('.select-user-group').css('display', 'block');
	});

	$('#newSessionRadio').on('click',function(){
		$('.new-session-group').css('display', 'block');
		$('.select-session-group').css('display', 'none');
	});

	$('#existingSessionRadio').on('click',function(){
		$('.new-session-group').css('display', 'none');
		$('.select-session-group').css('display', 'block');
	});

	$('#register').submit(function (e) {
		e.preventDefault();

		var valid = verifyRegister();

		if (valid) {
			$('.register').css('display', 'none');
			$('.editor').slideDown('slow');

			setTimeout(function(){
				editor.refresh();
			}, 500);
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
		}
	}

	return valid;
}