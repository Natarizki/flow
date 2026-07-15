// Call this from any page that wants FLOW's P2P cache to intercept its
// fetches. Needs a valid FLOW auth token (same one used for the
// dashboard API calls).
async function registerFlowServiceWorker(authToken) {
	if (!("serviceWorker" in navigator)) {
		console.warn("Service workers not supported in this browser");
		return null;
	}

	const registration = await navigator.serviceWorker.register("/flow-sw.js");
	await navigator.serviceWorker.ready;

	if (registration.active) {
		registration.active.postMessage({ type: "FLOW_AUTH_TOKEN", token: authToken });
	}

	// re-send token whenever the worker restarts (browsers can kill
	// idle service workers and respawn them without a page reload)
	navigator.serviceWorker.addEventListener("controllerchange", () => {
		if (navigator.serviceWorker.controller) {
			navigator.serviceWorker.controller.postMessage({ type: "FLOW_AUTH_TOKEN", token: authToken });
		}
	});

	return registration;
}
