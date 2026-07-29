package cognitive

import "encoding/json"

// DefaultWorkPacks returns the 6 work packs for Phase 17.
func DefaultWorkPacks() []*WorkPack {
	return []*WorkPack{
		virtualAssistantPack(),
		researchPack(),
		contentWritingPack(),
		codingAssistancePack(),
		socialMediaPack(),
		spreadsheetsPack(),
	}
}

func virtualAssistantPack() *WorkPack {
	return &WorkPack{
		ID:          "virtual-assistant",
		Name:        "Virtual Assistant",
		Description: "General-purpose virtual assistant tasks: scheduling, reminders, email drafting, data organization, and basic administrative work.",
		Version:     "1.0.0",
		Status:      WorkPackComplete,
		Priority:    1,
		Category:    "productivity",
		IntakeSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"task_type": {"type": "string", "enum": ["email_draft", "schedule", "reminder", "data_organize", "summary", "translation"]},
				"content": {"type": "string", "minLength": 1, "maxLength": 10000},
				"language": {"type": "string", "enum": ["ar", "en", "auto"]},
				"format": {"type": "string", "enum": ["text", "markdown", "json"]}
			},
			"required": ["task_type", "content"],
			"additionalProperties": false
		}`),
		Workflow: []WorkflowStep{
			{Name: "understand", Description: "Understand the user's request and identify task type", Risk: "low"},
			{Name: "prepare", Description: "Gather any missing information from user", Risk: "low"},
			{Name: "execute", Description: "Execute the task using appropriate tools", Risk: "low"},
			{Name: "review", Description: "Review output for quality and completeness", Risk: "low"},
			{Name: "deliver", Description: "Deliver the final output to the user", Risk: "low"},
		},
		RequiredTools: []string{"memory", "research"},
		QualityChecks: []QualityCheck{
			{Name: "completeness", Description: "Output addresses all parts of the request", Type: "llm", Criteria: "All requested items are present"},
			{Name: "language_match", Description: "Output matches requested language", Type: "automated", Criteria: "Language matches user preference"},
			{Name: "format_correct", Description: "Output is in requested format", Type: "automated", Criteria: "Format matches specification"},
		},
		DeliveryFormat:   "text",
		BeginnerGuide:    "This assistant helps you with everyday digital tasks. Just describe what you need in your own words (Arabic or English). I'll ask if I need more details, then do the work for you. You can review and ask for changes before accepting the final result.",
		EstimatedMinutes: 5,
	}
}

func researchPack() *WorkPack {
	return &WorkPack{
		ID:          "research",
		Name:        "Research & Information",
		Description: "Research tasks: topic analysis, competitor research, fact-finding, literature review, market analysis, and information synthesis.",
		Version:     "1.0.0",
		Status:      WorkPackComplete,
		Priority:    2,
		Category:    "knowledge",
		IntakeSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"topic": {"type": "string", "minLength": 3, "maxLength": 500},
				"depth": {"type": "string", "enum": ["quick", "standard", "deep"]},
				"language": {"type": "string", "enum": ["ar", "en", "auto"]},
				"sources": {"type": "integer", "minimum": 1, "maximum": 20},
				"format": {"type": "string", "enum": ["summary", "report", "bullet_points", "comparison_table"]}
			},
			"required": ["topic"],
			"additionalProperties": false
		}`),
		Workflow: []WorkflowStep{
			{Name: "clarify", Description: "Clarify research scope and requirements", Risk: "low"},
			{Name: "search", Description: "Search for relevant information", ToolName: "research", Risk: "low"},
			{Name: "analyze", Description: "Analyze and synthesize findings", Risk: "low"},
			{Name: "verify", Description: "Cross-check key facts and claims", Risk: "low"},
			{Name: "format", Description: "Format findings into requested output", Risk: "low"},
		},
		RequiredTools: []string{"research", "memory"},
		QualityChecks: []QualityCheck{
			{Name: "relevance", Description: "Findings are relevant to the topic", Type: "llm", Criteria: "All findings relate to the research topic"},
			{Name: "source_quality", Description: "Sources are credible", Type: "llm", Criteria: "Sources are from reputable origins"},
			{Name: "completeness", Description: "Research covers the requested scope", Type: "llm", Criteria: "All aspects of the topic are covered"},
		},
		DeliveryFormat:   "markdown",
		BeginnerGuide:    "This research tool helps you find and organize information on any topic. Just tell me what you want to research (e.g., 'Find information about starting an online store in Arabic'). I'll search, analyze, and present the findings in a clear report you can use.",
		EstimatedMinutes: 10,
	}
}

func contentWritingPack() *WorkPack {
	return &WorkPack{
		ID:          "content-writing",
		Name:        "Content Writing",
		Description: "Content creation tasks: blog posts, articles, social media captions, product descriptions, email newsletters, and marketing copy.",
		Version:     "1.0.0",
		Status:      WorkPackComplete,
		Priority:    3,
		Category:    "creative",
		IntakeSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"content_type": {"type": "string", "enum": ["blog_post", "article", "caption", "product_description", "email", "marketing_copy", "resume"]},
				"topic": {"type": "string", "minLength": 3, "maxLength": 500},
				"language": {"type": "string", "enum": ["ar", "en", "auto"]},
				"tone": {"type": "string", "enum": ["formal", "casual", "professional", "friendly", "persuasive"]},
				"word_count": {"type": "integer", "minimum": 50, "maximum": 5000},
				"audience": {"type": "string", "maxLength": 200},
				"keywords": {"type": "array", "items": {"type": "string"}, "maxItems": 10}
			},
			"required": ["content_type", "topic"],
			"additionalProperties": false
		}`),
		Workflow: []WorkflowStep{
			{Name: "brief", Description: "Understand content requirements and audience", Risk: "low"},
			{Name: "outline", Description: "Create content outline", Risk: "low"},
			{Name: "draft", Description: "Write the first draft", Risk: "low"},
			{Name: "review", Description: "Review for quality, tone, and accuracy", Risk: "low"},
			{Name: "finalize", Description: "Polish and deliver final content", Risk: "low"},
		},
		RequiredTools: []string{"research", "memory"},
		QualityChecks: []QualityCheck{
			{Name: "grammar", Description: "No grammatical errors", Type: "llm", Criteria: "Text is grammatically correct"},
			{Name: "tone_match", Description: "Tone matches specification", Type: "llm", Criteria: "Writing tone matches requested style"},
			{Name: "word_count", Description: "Word count within range", Type: "automated", Criteria: "Word count is within ±20% of target"},
			{Name: "originality", Description: "Content is original", Type: "llm", Criteria: "No plagiarism detected"},
		},
		DeliveryFormat:   "markdown",
		BeginnerGuide:    "This tool writes content for you. Tell me what kind of content you need (blog post, caption, email, etc.), what topic, and who it's for. I'll write a draft, you can review it and ask for changes. When you're happy, I'll deliver the final version. Don't worry about writing skills — I'll handle that!",
		EstimatedMinutes: 10,
	}
}

func codingAssistancePack() *WorkPack {
	return &WorkPack{
		ID:          "coding-assistance",
		Name:        "Coding Assistance",
		Description: "Coding tasks: code generation, bug fixing, code review, documentation, refactoring, and learning explanations.",
		Version:     "1.0.0",
		Status:      WorkPackComplete,
		Priority:    4,
		Category:    "technical",
		IntakeSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"task_type": {"type": "string", "enum": ["generate", "fix", "review", "document", "refactor", "explain", "test"]},
				"language": {"type": "string", "enum": ["python", "javascript", "go", "html_css", "sql", "bash", "auto"]},
				"description": {"type": "string", "minLength": 10, "maxLength": 5000},
				"code_input": {"type": "string", "maxLength": 50000},
				"explanation_language": {"type": "string", "enum": ["ar", "en"]}
			},
			"required": ["task_type", "description"],
			"additionalProperties": false
		}`),
		Workflow: []WorkflowStep{
			{Name: "understand", Description: "Understand the coding task and requirements", Risk: "low"},
			{Name: "analyze", Description: "Analyze existing code if provided", Risk: "low"},
			{Name: "implement", Description: "Write or fix the code", Risk: "low"},
			{Name: "test_mental", Description: "Mentally test the code for correctness", Risk: "low"},
			{Name: "explain", Description: "Explain the solution in beginner-friendly terms", Risk: "low"},
		},
		RequiredTools: []string{"memory"},
		QualityChecks: []QualityCheck{
			{Name: "syntax", Description: "Code has valid syntax", Type: "llm", Criteria: "Code would parse without syntax errors"},
			{Name: "logic", Description: "Code logic is correct", Type: "llm", Criteria: "Logic matches the requirements"},
			{Name: "explanation", Description: "Explanation is beginner-friendly", Type: "llm", Criteria: "Explanation uses simple language"},
		},
		DeliveryFormat:   "markdown",
		BeginnerGuide:    "This tool helps you with coding tasks. You can ask me to: write new code, fix bugs in existing code, review your code, explain how code works, or add documentation. Just describe what you need in plain words — you don't need to be a programmer! If you have existing code, paste it and tell me what's wrong or what you want to change.",
		EstimatedMinutes: 10,
	}
}

func socialMediaPack() *WorkPack {
	return &WorkPack{
		ID:          "social-media-management",
		Name:        "Social Media Management",
		Description: "Social media tasks: content planning, caption writing, hashtag research, posting schedules, and engagement analysis.",
		Version:     "1.0.0",
		Status:      WorkPackComplete,
		Priority:    5,
		Category:    "marketing",
		IntakeSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"platform": {"type": "string", "enum": ["instagram", "twitter", "facebook", "linkedin", "tiktok"]},
				"task_type": {"type": "string", "enum": ["caption", "content_plan", "hashtag_research", "schedule", "bio_write", "engagement_reply"]},
				"topic": {"type": "string", "minLength": 3, "maxLength": 500},
				"language": {"type": "string", "enum": ["ar", "en", "auto"]},
				"tone": {"type": "string", "enum": ["casual", "professional", "funny", "inspirational"]},
				"post_count": {"type": "integer", "minimum": 1, "maximum": 30}
			},
			"required": ["platform", "task_type", "topic"],
			"additionalProperties": false
		}`),
		Workflow: []WorkflowStep{
			{Name: "brief", Description: "Understand social media goals and brand voice", Risk: "low"},
			{Name: "research", Description: "Research trending content and hashtags", ToolName: "research", Risk: "low"},
			{Name: "create", Description: "Create content (captions, plans, etc.)", Risk: "low"},
			{Name: "optimize", Description: "Optimize for platform algorithm", Risk: "low"},
			{Name: "review", Description: "Review with user before publishing", Risk: "medium", Approval: true},
		},
		RequiredTools: []string{"research", "memory"},
		QualityChecks: []QualityCheck{
			{Name: "platform_fit", Description: "Content fits platform requirements", Type: "llm", Criteria: "Character limits, format, hashtags match platform"},
			{Name: "engagement", Description: "Content is engaging", Type: "llm", Criteria: "Captions are attention-grabbing and relevant"},
			{Name: "hashtag_relevance", Description: "Hashtags are relevant and trending", Type: "llm", Criteria: "Hashtags relate to content and have search volume"},
		},
		DeliveryFormat:   "markdown",
		BeginnerGuide:    "This tool helps you manage your social media presence. Tell me which platform (Instagram, Twitter, etc.) and what you need: captions for posts, a content plan for the week, hashtag suggestions, or help writing your bio. I'll create the content and show it to you before anything gets posted. Nothing goes live without your approval!",
		EstimatedMinutes: 10,
	}
}

func spreadsheetsPack() *WorkPack {
	return &WorkPack{
		ID:          "spreadsheets-reporting",
		Name:        "Spreadsheets & Reporting",
		Description: "Data tasks: spreadsheet creation, data analysis, report generation, chart descriptions, and data visualization guidance.",
		Version:     "1.0.0",
		Status:      WorkPackComplete,
		Priority:    6,
		Category:    "productivity",
		IntakeSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"task_type": {"type": "string", "enum": ["create_spreadsheet", "analyze_data", "generate_report", "chart_description", "formula_help", "data_cleanup"]},
				"data_description": {"type": "string", "minLength": 10, "maxLength": 5000},
				"language": {"type": "string", "enum": ["ar", "en", "auto"]},
				"format": {"type": "string", "enum": ["csv", "markdown_table", "json", "text_report"]},
				"data_input": {"type": "string", "maxLength": 50000}
			},
			"required": ["task_type", "data_description"],
			"additionalProperties": false
		}`),
		Workflow: []WorkflowStep{
			{Name: "understand", Description: "Understand data requirements", Risk: "low"},
			{Name: "structure", Description: "Design data structure and columns", Risk: "low"},
			{Name: "process", Description: "Process and organize data", Risk: "low"},
			{Name: "analyze", Description: "Perform analysis if requested", Risk: "low"},
			{Name: "deliver", Description: "Format and deliver output", Risk: "low"},
		},
		RequiredTools: []string{"memory"},
		QualityChecks: []QualityCheck{
			{Name: "data_accuracy", Description: "Data is accurate and consistent", Type: "llm", Criteria: "No obvious errors in data"},
			{Name: "format_valid", Description: "Output format is valid", Type: "automated", Criteria: "CSV/JSON/Markdown is well-formed"},
			{Name: "completeness", Description: "All requested data is included", Type: "llm", Criteria: "All columns and rows are present"},
		},
		DeliveryFormat:   "text",
		BeginnerGuide:    "This tool helps you work with data and spreadsheets. You can ask me to: create a spreadsheet with sample data, analyze data you provide, generate a report from numbers, explain how to create charts, or help with spreadsheet formulas. Just describe what data you have and what you want to do with it!",
		EstimatedMinutes: 10,
	}
}
