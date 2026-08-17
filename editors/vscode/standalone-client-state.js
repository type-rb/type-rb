"use strict";

function transitionStandaloneClient(project, running) {
	project.clientRunning = running;
	if (!running) {
		project.forwardedDocuments.clear();
		return false;
	}
	if (!project.clientStarted) {
		project.clientStarted = true;
		return false;
	}
	project.forwardedDocuments.clear();
	return true;
}

module.exports = { transitionStandaloneClient };
