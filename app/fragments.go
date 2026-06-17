package app

import (
	"context"
	"io"
	"net/http"
	"sort"

	"suppa-ahg-stack/common-golang/serverutil"
)

// FragmentID is a stable identifier for a fragment in the refresh planner.
type FragmentID string

// FragmentKind classifies how a fragment should be rendered.
type FragmentKind int

const (
	// FragmentRouteContent is the main route content (#page-content by default).
	FragmentRouteContent FragmentKind = iota
	// FragmentPageComponent is a registered layout/page component.
	FragmentPageComponent
	// FragmentCustom is rendered by a custom function.
	FragmentCustom
)

// CustomFragmentRenderFunc renders a custom fragment.
// The request and context carry the original target path and any FragmentInput.
type CustomFragmentRenderFunc func(r *http.Request, ctx context.Context, w io.Writer) error

// FragmentInput is request-scoped data passed to custom fragment renderers.
type FragmentInput struct {
	Search         string
	UserID         int64
	OrganisationID int64
	ContainerID    string
}

type fragmentInputKey struct{}

// WithFragmentInput stores FragmentInput in the context.
func WithFragmentInput(ctx context.Context, input FragmentInput) context.Context {
	return context.WithValue(ctx, fragmentInputKey{}, input)
}

// GetFragmentInput retrieves FragmentInput from the context.
func GetFragmentInput(ctx context.Context) FragmentInput {
	input, _ := ctx.Value(fragmentInputKey{}).(FragmentInput)
	return input
}

// RefreshPolicy controls which fragments are refreshed after an action or navigation.
type RefreshPolicy int

const (
	// RefreshPageOnly refreshes only the route content fragment.
	RefreshPageOnly RefreshPolicy = iota
	// RefreshPageAndNavbar refreshes the route content and navbar fragments.
	RefreshPageAndNavbar
	// RefreshPageAndLayout refreshes the route content, navbar, and layout fragments.
	RefreshPageAndLayout
)

// FragmentSpec describes a single fragment that can be rendered and published via SSE.
type FragmentSpec struct {
	ID          FragmentID
	Selector    string
	Kind        FragmentKind
	Invalidates []FragmentID
	Dominates   []FragmentID
	IsLayout    bool
	Order       int
	RenderFunc  CustomFragmentRenderFunc
}

// FragmentRegistry maps route keys to their fragment specifications.
type FragmentRegistry map[string]map[FragmentID]FragmentSpec

// FragmentPlanner computes refresh plans for SSE fragment updates.
type FragmentPlanner struct {
	registry FragmentRegistry
}

// CustomFragmentConfigurer is called once per route key while building the fragment registry.
// It can add custom fragments or mutate existing ones (for example to add dominance rules).
type CustomFragmentConfigurer func(routeKey string, fragments map[FragmentID]FragmentSpec)

// NewFragmentPlanner builds a planner from the app's routes, page components, and optional custom fragments.
//
// For each route in routes, a route-content fragment is created from route.TargetSelector.
// For each registered page component under the same route key, a page-component fragment is created.
// Layout components are refreshed together with the route content on navigation.
// configureFragments is called for each route key and may add or modify fragments.
func NewFragmentPlanner(
	routes map[string]serverutil.PageRoute,
	components map[string]map[string]RenderableComponent,
	configureFragments CustomFragmentConfigurer,
) *FragmentPlanner {
	registry := make(FragmentRegistry, len(routes))

	for routeKey, route := range routes {
		fragments := map[FragmentID]FragmentSpec{
			PageContentFragment: {
				ID:       PageContentFragment,
				Selector: route.TargetSelector,
				Kind:     FragmentRouteContent,
				Order:    100,
			},
		}

		// Register every page component so PublishFragmentForPathWithPlanner can find them by selector.
		var selectors []string
		for selector := range components[routeKey] {
			if selector == route.TargetSelector {
				continue
			}
			selectors = append(selectors, selector)
		}
		sort.Strings(selectors)

		for _, selector := range selectors {
			component := components[routeKey][selector]
			id := componentFragmentID(selector)
			order := componentFragmentOrder(selector, component.IsLayoutComponent())
			fragments[id] = FragmentSpec{
				ID:       id,
				Selector: component.GetTargetSelector(),
				Kind:     FragmentPageComponent,
				IsLayout: component.IsLayoutComponent(),
				Order:    order,
			}
		}

		if configureFragments != nil {
			configureFragments(routeKey, fragments)
		}

		registry[routeKey] = fragments
	}

	return &FragmentPlanner{registry: registry}
}

func componentFragmentID(selector string) FragmentID {
	switch selector {
	case "#navbar-content":
		return NavbarContentFragment
	case "#float-selection":
		return FloatSelectionFragment
	default:
		return FragmentID(selector)
	}
}

func componentFragmentOrder(selector string, isLayout bool) int {
	switch selector {
	case "#navbar-content":
		return 200
	case "#float-selection":
		return 300
	default:
		if isLayout {
			return 250
		}
		return 150
	}
}

// Plan computes the ordered list of fragments to refresh for a route starting from roots.
func (fp *FragmentPlanner) Plan(routeKey string, roots []FragmentID) []FragmentSpec {
	fragments := fp.registry[routeKey]
	if len(fragments) == 0 || len(roots) == 0 {
		return nil
	}

	scheduled := make(map[FragmentID]struct{}, len(roots))
	var visit func(id FragmentID)
	visit = func(id FragmentID) {
		if _, ok := scheduled[id]; ok {
			return
		}
		spec, ok := fragments[id]
		if !ok {
			return
		}
		scheduled[id] = struct{}{}
		for _, dep := range spec.Invalidates {
			visit(dep)
		}
	}

	for _, root := range roots {
		visit(root)
	}

	for id := range scheduled {
		spec, ok := fragments[id]
		if !ok {
			continue
		}
		for _, dominated := range spec.Dominates {
			delete(scheduled, dominated)
		}
	}

	planned := make([]FragmentSpec, 0, len(scheduled))
	for id := range scheduled {
		planned = append(planned, fragments[id])
	}

	sort.Slice(planned, func(i, j int) bool {
		if planned[i].Order == planned[j].Order {
			return planned[i].Selector < planned[j].Selector
		}
		return planned[i].Order < planned[j].Order
	})

	return planned
}

// FindBySelector returns the fragment registered for routeKey with the given CSS selector.
func (fp *FragmentPlanner) FindBySelector(routeKey, selector string) (FragmentSpec, bool) {
	fragments, ok := fp.registry[routeKey]
	if !ok {
		return FragmentSpec{}, false
	}

	for _, fragment := range fragments {
		if fragment.Selector == selector {
			return fragment, true
		}
	}

	return FragmentSpec{}, false
}

// Common fragment IDs used by the planner.
const (
	PageContentFragment    FragmentID = "page-content"
	NavbarContentFragment  FragmentID = "navbar-content"
	FloatSelectionFragment FragmentID = "float-selection"
)

// RootsForPolicy returns the fragment roots corresponding to a refresh policy.
func RootsForPolicy(policy RefreshPolicy) []FragmentID {
	switch policy {
	case RefreshPageAndNavbar:
		return []FragmentID{PageContentFragment, NavbarContentFragment}
	case RefreshPageAndLayout:
		return []FragmentID{PageContentFragment, NavbarContentFragment, FloatSelectionFragment}
	default:
		return []FragmentID{PageContentFragment}
	}
}
