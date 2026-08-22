package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

var (
	ErrNotFound         = errors.New("not found")
	ErrHandleRegistered = errors.New("handle already registered")
)

type Router struct {
	name     string
	path     []string
	children []*Router
	command  Command
	params   []string
}

func (r *Router) getChildren(name string) *Router {
	for _, child := range r.children {
		if child.name == name {
			return child
		}
	}
	return nil
}

func (r *Router) Completer(tokens ...string) []string {
	ss := make([]string, 0, 10)
	if len(tokens) == 0 {
		for _, child := range r.children {
			ss = append(ss, strings.Join(child.path, " "))
		}
		return ss
	}
	children := r.getChildren(tokens[0])
	if children == nil {
		token := tokens[0]
		for _, child := range r.children {
			if strings.HasPrefix(child.name, token) {
				ss = append(ss, strings.Join(child.path, " "))
			}
		}
		return ss
	}
	return children.Completer(tokens[1:]...)
}

func (r *Router) Usage() string {
	if len(r.path) <= 0 {
		return ""
	}
	var (
		sb strings.Builder
	)
	sb.WriteString("Usage: ")
	sb.WriteString(strings.Join(r.path, " "))
	if len(r.params) > 0 {
		for _, s := range r.params {
			sb.WriteString(" {" + s + "}")
		}
	}
	return sb.String()
}

func (r *Router) Handle(path string, command Command) (err error) {
	var (
		pos  int
		name string
	)
	if before, ok := strings.CutSuffix(path, "/"); ok {
		path = before
	}
	if after, ok := strings.CutPrefix(path, "/"); ok {
		path = after
	}
	if path == "" {
		r.command = command
		return nil
	}
	if path[0] == ':' {
		ss := strings.Split(path, "/")
		for _, s := range ss {
			r.params = append(r.params, strings.TrimPrefix(s, ":"))
		}
		r.command = command
		return nil
	}
	if pos = strings.IndexByte(path, '/'); pos > -1 {
		name = path[:pos]
		path = path[pos:]
	} else {
		name = path
		path = ""
	}
	if name == "-" {
		name = "app"
	}
	children := r.getChildren(name)
	if children == nil {
		children = newRouter(name)
		if len(r.path) == 0 {
			children.path = append(children.path, name)
		} else {
			children.path = append(children.path, r.path...)
			children.path = append(children.path, name)
		}
		r.children = append(r.children, children)
	}
	if children.command.Handle != nil {
		return fmt.Errorf("%w: /%s", ErrHandleRegistered, strings.Join(children.path, "/"))
	}
	return children.Handle(path, command)
}

func (r *Router) Lookup(tokens []string) (router *Router, args []string, err error) {
	if len(tokens) > 0 {
		children := r.getChildren(tokens[0])
		if children != nil {
			return children.Lookup(tokens[1:])
		}
	}
	if r.command.Handle == nil {
		err = ErrNotFound
		return
	}
	router = r
	args = tokens
	return
}

func (r *Router) String() string {
	var (
		sb       strings.Builder
		width    int
		maxWidth int
		walkFunc func(router *Router) []commander
	)
	walkFunc = func(router *Router) []commander {
		vs := make([]commander, 0, 5)
		if router.command.Handle != nil {
			vs = append(vs, commander{
				Name:        router.name,
				Path:        strings.Join(router.path, " "),
				Description: router.command.Description,
			})
		} else {
			if len(router.children) > 0 {
				for _, child := range router.children {
					vs = append(vs, walkFunc(child)...)
				}
			}
		}
		return vs
	}
	vs := walkFunc(r)
	for _, v := range vs {
		width = runewidth.StringWidth(v.Path)
		if width > maxWidth {
			maxWidth = width
		}
	}
	for _, v := range vs {
		sb.WriteString(fmt.Sprintf("%-"+strconv.Itoa(maxWidth+4)+"s %s\n", v.Path, v.Description))
	}
	return sb.String()
}

func newRouter(name string) *Router {
	return &Router{
		name:     name,
		path:     make([]string, 0, 4),
		params:   make([]string, 0, 4),
		children: make([]*Router, 0, 10),
	}
}
