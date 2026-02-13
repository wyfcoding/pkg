package docs

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ServiceDoc struct {
	Name        string       `json:"name"`
	Package     string       `json:"package"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Methods     []MethodDoc  `json:"methods"`
	Messages    []MessageDoc `json:"messages"`
	GeneratedAt time.Time    `json:"generated_at"`
}

type MethodDoc struct {
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	RequestType    string      `json:"request_type"`
	ResponseType   string      `json:"response_type"`
	IsClientStream bool        `json:"is_client_stream"`
	IsServerStream bool        `json:"is_server_stream"`
	HTTPMapping    HTTPMapping `json:"http_mapping"`
	Deprecated     bool        `json:"deprecated"`
	DeprecationMsg string      `json:"deprecation_message,omitempty"`
}

type HTTPMapping struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Body    string            `json:"body,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

type MessageDoc struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Fields      []FieldDoc  `json:"fields"`
	IsEnum      bool        `json:"is_enum"`
	EnumValues  []EnumValue `json:"enum_values,omitempty"`
}

type FieldDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Number      int    `json:"number"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Repeated    bool   `json:"repeated"`
	Optional    bool   `json:"optional"`
	Deprecated  bool   `json:"deprecated"`
}

type EnumValue struct {
	Name        string `json:"name"`
	Number      int    `json:"number"`
	Description string `json:"description"`
}

type APIDocumentation struct {
	Services    []ServiceDoc `json:"services"`
	GeneratedAt time.Time    `json:"generated_at"`
	Version     string       `json:"version"`
}

type DocGenerator struct {
	outputDir string
	format    string
}

type GeneratorOption func(*DocGenerator)

func WithOutputDir(dir string) GeneratorOption {
	return func(g *DocGenerator) {
		g.outputDir = dir
	}
}

func WithFormat(format string) GeneratorOption {
	return func(g *DocGenerator) {
		g.format = format
	}
}

func NewDocGenerator(opts ...GeneratorOption) *DocGenerator {
	g := &DocGenerator{
		outputDir: "./docs",
		format:    "markdown",
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

func (g *DocGenerator) GenerateFromProto(protoPath string) (*APIDocumentation, error) {
	file, err := os.Open(protoPath)
	if err != nil {
		return nil, fmt.Errorf("open proto file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("read proto file: %w", err)
	}

	return g.parseProto(string(content), protoPath)
}

func (g *DocGenerator) parseProto(content, filePath string) (*APIDocumentation, error) {
	doc := &APIDocumentation{
		GeneratedAt: time.Now(),
		Version:     "1.0.0",
	}

	packageName := g.extractPackage(content)
	services := g.extractServices(content)
	messages := g.extractMessages(content)

	for _, svc := range services {
		serviceDoc := ServiceDoc{
			Name:        svc.Name,
			Package:     packageName,
			Version:     "v1",
			Description: svc.Description,
			Methods:     svc.Methods,
			GeneratedAt: time.Now(),
		}

		for _, msg := range messages {
			if strings.Contains(msg.Name, svc.Name) ||
				g.isMethodType(msg.Name, svc.Methods) {
				serviceDoc.Messages = append(serviceDoc.Messages, msg)
			}
		}

		doc.Services = append(doc.Services, serviceDoc)
	}

	return doc, nil
}

func (g *DocGenerator) isMethodType(msgName string, methods []MethodDoc) bool {
	for _, m := range methods {
		if m.RequestType == msgName || m.ResponseType == msgName {
			return true
		}
	}
	return false
}

type serviceInfo struct {
	Name        string
	Description string
	Methods     []MethodDoc
}

func (g *DocGenerator) extractServices(content string) []serviceInfo {
	var services []serviceInfo

	serviceRegex := regexp.MustCompile(`service\s+(\w+)\s*\{([^}]*)\}`)
	matches := serviceRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		svc := serviceInfo{
			Name: match[1],
		}

		svc.Methods = g.extractMethods(match[2])
		services = append(services, svc)
	}

	return services
}

func (g *DocGenerator) extractMethods(serviceBody string) []MethodDoc {
	var methods []MethodDoc

	methodRegex := regexp.MustCompile(`(\/\/[^\n]*\n\s*)?(rpc\s+(\w+)\s*\(\s*(stream\s+)?(\w+)\s*\)\s*returns\s*\(\s*(stream\s+)?(\w+)\s*\))`)
	matches := methodRegex.FindAllStringSubmatch(serviceBody, -1)

	for _, match := range matches {
		method := MethodDoc{
			Name:           match[3],
			Description:    strings.TrimPrefix(strings.TrimPrefix(match[1], "//"), " "),
			RequestType:    match[5],
			ResponseType:   match[7],
			IsClientStream: match[4] != "",
			IsServerStream: match[6] != "",
		}

		method.Description = strings.TrimSpace(method.Description)
		methods = append(methods, method)
	}

	return methods
}

func (g *DocGenerator) extractPackage(content string) string {
	pkgRegex := regexp.MustCompile(`package\s+([\w.]+)`)
	match := pkgRegex.FindStringSubmatch(content)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func (g *DocGenerator) extractMessages(content string) []MessageDoc {
	var messages []MessageDoc

	messageRegex := regexp.MustCompile(`message\s+(\w+)\s*\{([^}]*)\}`)
	matches := messageRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		msg := MessageDoc{
			Name:   match[1],
			Fields: g.extractFields(match[2]),
		}
		messages = append(messages, msg)
	}

	enumRegex := regexp.MustCompile(`enum\s+(\w+)\s*\{([^}]*)\}`)
	enumMatches := enumRegex.FindAllStringSubmatch(content, -1)

	for _, match := range enumMatches {
		msg := MessageDoc{
			Name:       match[1],
			IsEnum:     true,
			EnumValues: g.extractEnumValues(match[2]),
		}
		messages = append(messages, msg)
	}

	return messages
}

func (g *DocGenerator) extractFields(messageBody string) []FieldDoc {
	var fields []FieldDoc

	fieldRegex := regexp.MustCompile(`(\/\/[^\n]*\n\s*)?(repeated\s+|optional\s+)?(\w+)\s+(\w+)\s*=\s*(\d+)`)
	matches := fieldRegex.FindAllStringSubmatch(messageBody, -1)

	for _, match := range matches {
		field := FieldDoc{
			Description: strings.TrimSpace(strings.TrimPrefix(match[1], "//")),
			Type:        match[3],
			Name:        match[4],
			Repeated:    strings.TrimSpace(match[2]) == "repeated",
			Optional:    strings.TrimSpace(match[2]) == "optional",
		}

		fmt.Sscanf(match[5], "%d", &field.Number)
		fields = append(fields, field)
	}

	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Number < fields[j].Number
	})

	return fields
}

func (g *DocGenerator) extractEnumValues(enumBody string) []EnumValue {
	var values []EnumValue

	valueRegex := regexp.MustCompile(`(\w+)\s*=\s*(\d+)`)
	matches := valueRegex.FindAllStringSubmatch(enumBody, -1)

	for _, match := range matches {
		if match[1] == "UNSPECIFIED" {
			continue
		}

		value := EnumValue{
			Name: match[1],
		}
		fmt.Sscanf(match[2], "%d", &value.Number)
		values = append(values, value)
	}

	return values
}

func (g *DocGenerator) GenerateMarkdown(doc *APIDocumentation) (string, error) {
	var sb strings.Builder

	sb.WriteString("# API Documentation\n\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n\n", doc.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString("---\n\n")

	for _, svc := range doc.Services {
		sb.WriteString(fmt.Sprintf("## %s\n\n", svc.Name))
		sb.WriteString(fmt.Sprintf("**Package:** `%s`\n\n", svc.Package))

		if svc.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", svc.Description))
		}

		sb.WriteString("### Methods\n\n")

		for _, method := range svc.Methods {
			sb.WriteString(fmt.Sprintf("#### %s\n\n", method.Name))

			if method.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", method.Description))
			}

			sb.WriteString("| Property | Value |\n")
			sb.WriteString("|----------|-------|\n")
			sb.WriteString(fmt.Sprintf("| Request | `%s` |\n", method.RequestType))
			sb.WriteString(fmt.Sprintf("| Response | `%s` |\n", method.ResponseType))

			if method.IsClientStream || method.IsServerStream {
				streamType := ""
				if method.IsClientStream {
					streamType += "client "
				}
				if method.IsServerStream {
					streamType += "server "
				}
				sb.WriteString(fmt.Sprintf("| Streaming | %s|\n", streamType))
			}

			sb.WriteString("\n")
		}

		if len(svc.Messages) > 0 {
			sb.WriteString("### Messages\n\n")

			for _, msg := range svc.Messages {
				if msg.IsEnum {
					sb.WriteString(fmt.Sprintf("#### %s (Enum)\n\n", msg.Name))
					sb.WriteString("| Name | Number |\n")
					sb.WriteString("|------|--------|\n")
					for _, v := range msg.EnumValues {
						sb.WriteString(fmt.Sprintf("| %s | %d |\n", v.Name, v.Number))
					}
					sb.WriteString("\n")
				} else {
					sb.WriteString(fmt.Sprintf("#### %s\n\n", msg.Name))
					sb.WriteString("| Field | Type | Number | Description |\n")
					sb.WriteString("|-------|------|--------|-------------|\n")
					for _, f := range msg.Fields {
						typeStr := f.Type
						if f.Repeated {
							typeStr = "[]" + typeStr
						}
						sb.WriteString(fmt.Sprintf("| %s | `%s` | %d | %s |\n", f.Name, typeStr, f.Number, f.Description))
					}
					sb.WriteString("\n")
				}
			}
		}

		sb.WriteString("---\n\n")
	}

	return sb.String(), nil
}

func (g *DocGenerator) GenerateOpenAPI(doc *APIDocumentation) (string, error) {
	openapi := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "API Documentation",
			"version":     doc.Version,
			"description": fmt.Sprintf("Generated at %s", doc.GeneratedAt.Format(time.RFC3339)),
		},
		"paths": make(map[string]any),
		"components": map[string]any{
			"schemas": make(map[string]any),
		},
	}

	paths := openapi["paths"].(map[string]any)
	components := openapi["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)

	for _, svc := range doc.Services {
		for _, method := range svc.Methods {
			path := fmt.Sprintf("/%s/%s", strings.ToLower(svc.Name), strings.ToLower(method.Name))

			paths[path] = map[string]any{
				"post": map[string]any{
					"summary":     method.Description,
					"operationId": fmt.Sprintf("%s_%s", svc.Name, method.Name),
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"$ref": fmt.Sprintf("#/components/schemas/%s", method.RequestType),
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Success",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"$ref": fmt.Sprintf("#/components/schemas/%s", method.ResponseType),
									},
								},
							},
						},
					},
				},
			}
		}

		for _, msg := range svc.Messages {
			if !msg.IsEnum {
				properties := make(map[string]any)
				required := make([]string, 0)

				for _, f := range msg.Fields {
					properties[f.Name] = map[string]any{
						"type":        g.protoTypeToOpenAPI(f.Type),
						"description": f.Description,
					}

					if f.Required {
						required = append(required, f.Name)
					}
				}

				schemas[msg.Name] = map[string]any{
					"type":       "object",
					"properties": properties,
				}

				if len(required) > 0 {
					schemas[msg.Name].(map[string]any)["required"] = required
				}
			}
		}
	}

	result, err := json.MarshalIndent(openapi, "", "  ")
	return string(result), err
}

func (g *DocGenerator) protoTypeToOpenAPI(protoType string) string {
	typeMap := map[string]string{
		"string":                    "string",
		"int32":                     "integer",
		"int64":                     "integer",
		"uint32":                    "integer",
		"uint64":                    "integer",
		"float":                     "number",
		"double":                    "number",
		"bool":                      "boolean",
		"bytes":                     "string",
		"google.protobuf.Timestamp": "string",
		"google.protobuf.Duration":  "string",
	}

	if t, ok := typeMap[protoType]; ok {
		return t
	}

	return "object"
}

func (g *DocGenerator) SaveDocumentation(doc *APIDocumentation, filename string) error {
	if err := os.MkdirAll(g.outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	var content string
	var err error

	switch g.format {
	case "json":
		var data []byte
		data, err = json.MarshalIndent(doc, "", "  ")
		content = string(data)
	case "openapi":
		content, err = g.GenerateOpenAPI(doc)
	default:
		content, err = g.GenerateMarkdown(doc)
	}

	if err != nil {
		return fmt.Errorf("generate documentation: %w", err)
	}

	outputPath := filepath.Join(g.outputDir, filename)
	return os.WriteFile(outputPath, []byte(content), 0644)
}

func (g *DocGenerator) GenerateAll(protoDir string) error {
	var allDocs APIDocumentation
	allDocs.GeneratedAt = time.Now()
	allDocs.Version = "1.0.0"

	err := filepath.Walk(protoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".proto") && !strings.Contains(path, "error_reason") {
			doc, err := g.GenerateFromProto(path)
			if err != nil {
				return fmt.Errorf("process %s: %w", path, err)
			}

			allDocs.Services = append(allDocs.Services, doc.Services...)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("walk proto directory: %w", err)
	}

	return g.SaveDocumentation(&allDocs, "api_documentation.md")
}
