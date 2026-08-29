package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/type-rb/type-rb/internal/project"
)

type initTemplateFile struct {
	Path   string
	Source string
}

func initTemplateFiles(config *project.Config, template string) []initTemplateFile {
	if config == nil || template != "web" {
		return nil
	}
	root := config.SourcePath()
	return []initTemplateFile{
		{
			Path: filepath.Join(root, "main.trb"),
			Source: `import { serve } from trb/web

def main()
	serve()
	return
end
`,
		},
		{
			Path: filepath.Join(root, "routes", "index.trb"),
			Source: `import { Context, Response, json } from trb/web

record HelloResponse
	message: String
end

def get(_context: Context): Response
	return json(HelloResponse.new(message: "Hello, TypeRB!"))
end
`,
		},
		{
			Path: filepath.Join(root, "routes", "_middleware.trb"),
			Source: `import { Context, Next, Response } from trb/web
import { Middleware, compose } from trb/web/middleware
import trb/web/middleware/logger
import trb/web/middleware/request_id

MIDDLEWARES: Array<Middleware> := [
	RequestID.middleware(),
	Logger.middleware(),
]

def call(context: Context, next_handler: Next): Response
	return compose(context, next_handler, MIDDLEWARES)
end
`,
		},
	}
}

func checkInitTemplateTargets(files []initTemplateFile) error {
	for _, file := range files {
		if _, err := os.Stat(file.Path); err == nil {
			return fmt.Errorf("project template would overwrite %s", file.Path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeInitTemplate(files []initTemplateFile) error {
	for _, file := range files {
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return err
		}
		output, err := os.OpenFile(file.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		if _, err := output.WriteString(file.Source); err != nil {
			output.Close()
			return err
		}
		if err := output.Close(); err != nil {
			return err
		}
	}
	return nil
}
