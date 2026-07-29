# Image-generation skill

Generate through the core image provider abstraction only. Validate MIME type, byte size, dimensions, and hash before storing an artifact. The skill never publishes its output and never receives a provider secret in model-visible input. The provider implementation is added in Phase 11.
