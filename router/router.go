package router

import (
	"github.com/raythorn/falcon/context"
	"github.com/raythorn/falcon/log"
	"github.com/raythorn/falcon/oss"
	"net/http"
)

type Handler func(*context.Context)
type Midware func(*context.Context) bool

type Router interface {

	// Add midware to router, these handler will called before every request handler.
	// If more than one midware added, the will be called with their add order.
	// If Midware return false, this session will be intercepted, and will return immediately，
	// all following midwares and handlers will not be executed
	Use(Midware)

	// Group add a groupped router, all router has a same prefix, and should use GGet/GPut/GPatch...
	// for add groupped router, and GSub can add a sub-group for current group
	Group(string, ...interface{}) *Group

	// Oss add a object storage sevice, which can download and upload objects(file/image...)
	Oss(string, *oss.Oss)

	// Get adds a route for a HTTP GET request to the specified matching pattern.
	Get(string, Handler)

	// Patch adds a route for a HTTP PATCH request to the specified matching pattern.
	Patch(string, Handler)

	// Put adds a route for a HTTP PUT request to the specified matching pattern.
	Put(string, Handler)

	// Post adds a route for a HTTP POST request to the specified matching pattern.
	Post(string, Handler)

	// Delete adds a route for a HTTP DELETE request to the specified matching pattern.
	Delete(string, Handler)

	// Head adds a route for a HTTP HEAD request to the specified matching pattern.
	Head(string, Handler)

	// Options adds a route for a HTTP OPTIONS request to the specified matching pattern.
	Options(string, Handler)

	// Any adds a route for any HTTP method request to the specified matching pattern.
	Any(string, Handler)

	// NotFound sets the handlers that are called when a no route matches a request. Throws a basic 404 by default.
	NotFound(Handler)

	// NotAllowed sets the handler that are called when a not allowed http method request
	NotAllowed(Handler)

	// Handle is the entry point for routing.
	Handle(http.ResponseWriter, *http.Request)
}

type router struct {
	route      *Group
	group      *Group
	midwares   []Midware
	notfound   Handler
	notallowed Handler
}

func New() Router {

	r := &router{
		route:      newGroup(),
		group:      newGroup(),
		midwares:   make([]Midware, 0),
		notfound:   nil,
		notallowed: nil,
	}

	r.route.pattern = "/"

	return r
}

func (r *router) Use(midware Midware) {
	r.midwares = append(r.midwares, midware)
}

func (r *router) Group(prefix string, args ...interface{}) *Group {

	path := cleanPath(prefix)

	return r.group.group(path, args...)
}

func (r *router) Oss(pattern string, _oss *oss.Oss) {

	route := r.route.insert("GET", pattern, oss.Download)
	route.oss = _oss

	route.actions["POST"] = oss.Upload
}

func (r *router) Get(pattern string, handler Handler) {

	r.route.insert("GET", pattern, handler)
}

func (r *router) Patch(pattern string, handler Handler) {
	r.route.insert("PATCH", pattern, handler)
}

func (r *router) Put(pattern string, handler Handler) {
	r.route.insert("PUT", pattern, handler)
}

func (r *router) Post(pattern string, handler Handler) {
	r.route.insert("POST", pattern, handler)
}

func (r *router) Delete(pattern string, handler Handler) {
	r.route.insert("DELETE", pattern, handler)
}

func (r *router) Head(pattern string, handler Handler) {
	r.route.insert("HEAD", pattern, handler)
}

func (r *router) Options(pattern string, handler Handler) {
	r.route.insert("OPTIONS", pattern, handler)
}

func (r *router) Any(pattern string, handler Handler) {
	r.route.insert("ANY", pattern, handler)
}

func (r *router) NotFound(handler Handler) {
	r.notfound = handler
}

func (r *router) NotAllowed(handler Handler) {
	r.notallowed = handler
}

func (r *router) Handle(rw http.ResponseWriter, req *http.Request) {

	r.recovery()

	ctx := context.New()
	ctx.Reset(rw, req)

	// log.Printf("URI: %s", ctx.URI())
	// log.Printf("PATH: %s", ctx.URL())

	//Call all midware first
	if len(r.midwares) > 0 {
		for _, midware := range r.midwares {
			if !midware(ctx) {
				return
			}
		}
	}

	//Search Group
	route := r.group.match(ctx)
	if route != nil {
		var handler Handler = nil
		var ok bool = false

		// Check route exist or not, if not eixst return with notfound handler
		if handler, ok = route.actions[ctx.Method()]; !ok {
			if r.notfound != nil {
				r.notfound(ctx)
			} else {
				http.NotFound(rw, req)
			}

			return
		}

		if route.group != nil && len(route.group.before) > 0 {
			for _, midware := range route.group.before {
				if !midware(ctx) {
					return
				}
			}
		}

		if route.oss != nil {
			ctx.Set(oss.OssRootKey, route.oss.Root())
			if route.oss.Archive() != nil {
				ctx.Set(oss.OssPathKey, route.oss.Archive().Path(ctx))
			}
		}

		handler(ctx)

		if route.group != nil && len(route.group.after) > 0 {
			for _, midware := range route.group.after {
				if !midware(ctx) {
					return
				}
			}
		}

		return
	}

	// Search route
	route = r.route.match(ctx)
	if route != nil {

		if h, ok := route.actions[ctx.Method()]; ok {
			if route.oss != nil {
				ctx.Set(oss.OssRootKey, route.oss.Root())
				ctx.Set(oss.OssPathKey, route.oss.Archive().Path(ctx))
			}

			h(ctx)
		}

		return
	}

	//Not found
	if r.notfound != nil {
		r.notfound(ctx)
	} else {
		http.NotFound(rw, req)
	}
}

func (r *router) recovery() {
	defer func() {
		if err := recover(); err != nil {
			log.Error("%s\n", err)
		}
	}()
}
