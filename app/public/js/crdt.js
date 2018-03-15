$(document).ready(function(){
	editor.on('change', function(cm, change){
		handleChange(change);
	});
});

function handleChange(change) {
	const line = change.from.line;
	const pos = change.from.ch;

	if (change.origin == "+input") {
		handleInput(line, pos, change.text[0]);
	} else if (change.origin == "+delete") {
		handleRemove(line, pos, change.removed[0]);
	}
}

function handleInput(line, pos, inputChar) {
	console.log("Observed input at line: " + line + " pos: " + pos + "char: " + inputChar);
}

function handleRemove(line, pos, removeChar) {
	console.log("Observed remove at line: " + line + " pos: " + pos + "char: " + removeChar);
}