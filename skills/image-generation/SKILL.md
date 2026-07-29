# Image-generation skill

Generate only through the core `images.generate` tool and its OpenAI-compatible provider abstraction. The builtin Skill handler routes to the Tool Registry, where policy and approval are enforced. Validate prompt, cost budget, dimensions/aspect ratio, optional reference artifacts, MIME type, decoded dimensions, byte limit, and SHA-256 before atomic artifact storage.

The generated artifact has no publishing capability. Provider credentials remain outside model-visible input and `PENDING_EXTERNAL_VALIDATION`.
