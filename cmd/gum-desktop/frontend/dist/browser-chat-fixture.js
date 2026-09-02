// Browser Mock OpenAI-compatible fixture: mirrors the canonical chat Adapter
// behavior for the shared frontend contract without touching real networks.
export function createFixtureChatAdapter(options = {}) {
	// Responses are queued in order; the final one repeats when exhausted.
	const responses = options.responses ?? [{ assistantText: "Browser fixture response.", finishReason: "stop", usage: { inputTokens: 1, outputTokens: 1, totalTokens: 2 }, providerRequestId: "chatcmpl-browser-default" }];
	const secrets = options.secrets;
	const requests = options.requests ?? [];
	return {
		generate(connection, request) {
			requests.push({
				authorization: `Bearer ${secrets.resolve(connection.apiKeyRef)}`,
				baseUrl: connection.baseUrl,
				model: connection.providerModelId,
				instructionsRole: connection.dialect ?? "developer",
				instructions: request.instructions?.map((part) => part.text).join("\n") ?? "",
				messages: request.messages.map((message) => ({ role: message.role, text: message.parts.map((part) => part.text).join("\n") })),
				config: request.config,
			});
			const response = responses.length > 1 ? responses.shift() : responses[0];
			if (!response || response.error) throw new Error(response?.error ?? "fixture model call failed");
			return {
				assistant: { role: "assistant", parts: [{ kind: "text", text: response.assistantText }] },
				finishReason: response.finishReason ?? "stop",
				usage: response.usage ?? { inputTokens: 12, outputTokens: 7, totalTokens: 19 },
				providerRequestId: response.providerRequestId ?? "chatcmpl-fixture-1",
			};
		},
	};
}
