package app

import (
	"testing"

	"suppa-ahg-stack/common-golang/serverutil"
)

func TestPlanFragmentsPrunesDominatedDescendants(t *testing.T) {
	planner := &FragmentPlanner{
		registry: FragmentRegistry{
			"/organisations/": {
				PageContentFragment: {
					ID:          PageContentFragment,
					Selector:    "#page-content",
					Invalidates: []FragmentID{"child-fragment"},
					Dominates:   []FragmentID{"child-fragment"},
					Order:       100,
				},
				"child-fragment": {
					ID:       "child-fragment",
					Selector: "#child-fragment",
					Order:    200,
				},
			},
		},
	}

	planned := planner.Plan("/organisations/", []FragmentID{PageContentFragment})
	if len(planned) != 1 {
		t.Fatalf("expected 1 fragment after dominance pruning, got %d", len(planned))
	}
	if planned[0].ID != PageContentFragment {
		t.Fatalf("expected page-content fragment, got %s", planned[0].ID)
	}
}

func TestPlanFragmentsSortsByOrder(t *testing.T) {
	planner := &FragmentPlanner{
		registry: FragmentRegistry{
			"/": {
				NavbarContentFragment: {
					ID:       NavbarContentFragment,
					Selector: "#navbar-content",
					Order:    200,
				},
				PageContentFragment: {
					ID:       PageContentFragment,
					Selector: "#page-content",
					Order:    100,
				},
			},
		},
	}

	planned := planner.Plan("/", []FragmentID{NavbarContentFragment, PageContentFragment})
	if len(planned) != 2 {
		t.Fatalf("expected 2 fragments, got %d", len(planned))
	}
	if planned[0].ID != PageContentFragment || planned[1].ID != NavbarContentFragment {
		t.Fatalf("unexpected fragment order: %s, %s", planned[0].ID, planned[1].ID)
	}
}

func TestFindFragmentBySelector(t *testing.T) {
	planner := &FragmentPlanner{
		registry: FragmentRegistry{
			"/": {
				NavbarContentFragment: {
					ID:       NavbarContentFragment,
					Selector: "#navbar-content",
				},
			},
		},
	}

	fragment, ok := planner.FindBySelector("/", "#navbar-content")
	if !ok {
		t.Fatal("expected fragment selector to be found")
	}
	if fragment.ID != NavbarContentFragment {
		t.Fatalf("expected navbar-content fragment, got %s", fragment.ID)
	}
}

func TestPlanFragmentsIncludesCustomFragmentOnly(t *testing.T) {
	planner := &FragmentPlanner{
		registry: FragmentRegistry{
			"/admin/users/": {
				"access-modal-list-apps": {
					ID:       "access-modal-list-apps",
					Selector: "#access-modal-list-apps",
					Kind:     FragmentCustom,
					Order:    400,
				},
				NavbarContentFragment: {
					ID:       NavbarContentFragment,
					Selector: "#navbar-content",
					Kind:     FragmentPageComponent,
					Order:    200,
				},
			},
		},
	}

	planned := planner.Plan("/admin/users/", []FragmentID{"access-modal-list-apps"})
	if len(planned) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(planned))
	}
	if planned[0].ID != "access-modal-list-apps" || planned[0].Kind != FragmentCustom {
		t.Fatalf("expected custom access-modal-list-apps fragment, got %+v", planned[0])
	}
}

func TestWithFragmentInputRoundTrip(t *testing.T) {
	input := FragmentInput{
		Search:         "Root",
		UserID:         7,
		OrganisationID: 1,
		ContainerID:    "access-modal-list-orgs",
	}

	ctx := WithFragmentInput(t.Context(), input)
	got := GetFragmentInput(ctx)
	if got != input {
		t.Fatalf("expected fragment input %+v, got %+v", input, got)
	}
}

func TestMatchRouteStatic(t *testing.T) {
	routes := map[string]serverutil.PageRoute{
		"/organisations/": {},
	}

	key, _, ok := MatchRoute(routes, "/organisations/")
	if !ok || key != "/organisations/" {
		t.Fatalf("expected static match, got key=%s ok=%t", key, ok)
	}
}

func TestMatchRouteDynamic(t *testing.T) {
	routes := map[string]serverutil.PageRoute{
		"/organisations/{id}/apps/": {},
	}

	key, _, ok := MatchRoute(routes, "/organisations/42/apps/")
	if !ok || key != "/organisations/{id}/apps/" {
		t.Fatalf("expected dynamic match, got key=%s ok=%t", key, ok)
	}

	_, _, ok = MatchRoute(routes, "/organisations/abc/apps/")
	if ok {
		t.Fatal("expected no match for non-numeric dynamic segment")
	}
}
