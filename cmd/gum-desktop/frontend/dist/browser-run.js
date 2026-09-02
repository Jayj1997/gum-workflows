function normalize(value) {
	if (Array.isArray(value)) return value.map(normalize);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.keys(value).sort().map((key) => [key, normalize(value[key])]));
	}
	return value;
}

// Browser Mock Revision identity mirrors the Product Application semantics.
export function productRevisionKey(content) {
	const semantic = structuredClone(content);
	delete semantic.displayName;
	delete semantic.description;
	delete semantic.view;
	for (const node of semantic.nodes ?? []) {
		delete node.displayName;
		delete node.description;
		delete node.presentation;
		if (Array.isArray(node.dependsOn)) node.dependsOn.sort();
	}
	semantic.nodes?.sort((left, right) => left.id.localeCompare(right.id));
	return JSON.stringify(normalize(semantic));
}

// failBrowserRun applies the same terminal shape as the Desktop Product
// Application after execution has crossed the running boundary.
export function failBrowserRun(run, details, finishedAt = new Date().toISOString()) {
	run.status = "failed";
	run.error = structuredClone(details);
	run.finishedAt = finishedAt;
	for (const nodeRun of run.nodeRuns ?? []) {
		if (nodeRun.status !== "running") continue;
		nodeRun.status = "failed";
		nodeRun.finishedAt = finishedAt;
		nodeRun.diagnostics = { ...(nodeRun.diagnostics ?? {}), error: structuredClone(details) };
	}
	return run;
}

// interruptBrowserRuns reconciles in-memory Browser Mock runs when its
// workspace is opened again. Browser Mock remains intentionally non-durable.
export function interruptBrowserRuns(runs, interruptedAt = new Date().toISOString()) {
	const details = {
		kind: "unknown-outcome",
		code: "application-interrupted",
		message: "the Browser Mock workspace reopened before the Provider outcome was recorded",
		userAction: "this Run cannot Resume; inspect successful Artifacts and start a new Run only if it is safe",
	};
	for (const run of runs.values()) {
		if (run.status !== "running") continue;
		run.status = "interrupted";
		run.error = structuredClone(details);
		run.finishedAt = interruptedAt;
		for (const nodeRun of run.nodeRuns ?? []) {
			if (nodeRun.status !== "running") continue;
			nodeRun.status = "unknown-outcome";
			nodeRun.finishedAt = interruptedAt;
			nodeRun.diagnostics = { ...(nodeRun.diagnostics ?? {}), error: structuredClone(details) };
		}
	}
}
