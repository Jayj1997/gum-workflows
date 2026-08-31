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
