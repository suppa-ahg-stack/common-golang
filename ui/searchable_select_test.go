package ui

import (
	"context"
	"strings"
	"testing"
)

func renderSearchableSelectToString(t *testing.T, ctx context.Context, props SearchableSelectProps) string {
	t.Helper()
	return renderToString(t, ctx, SearchableSelect(props))
}

func TestSearchableSelectRendersRootAndDataAttributes(t *testing.T) {
	ctx := context.Background()
	props := SearchableSelectProps{
		ID:              "org-parent-search",
		Name:            "parent_org_id",
		Label:           "Parent organisation",
		Placeholder:     "Search organisation...",
		SearchAction:    "search-organisation-options",
		SearchParamName: "search",
		ResultSelector:  "#organisation-search-results",
		MinSearchLength: 1,
		DebounceMs:      200,
		MaxVisibleItems: 10,
		MaxItems:        20,
		MultiSelect:     false,
		Selected:        []SearchableSelectOption{{ID: "5", Label: "Alpha"}},
		NoResultsLabel:  "No results",
		ClearLabel:      "Clear",
	}

	html := renderSearchableSelectToString(t, ctx, props)

	required := []string{
		`id="org-parent-search"`,
		`x-data="searchableSelect"`,
		`data-name="parent_org_id"`,
		`data-search-action="search-organisation-options"`,
		`data-search-param-name="search"`,
		`data-result-selector="#organisation-search-results"`,
		`data-min-search-length="1"`,
		`data-debounce-ms="200"`,
		`data-max-visible-items="10"`,
		`data-max-items="20"`,
		`data-multiselect="false"`,
		`data-selected="[{&#34;id&#34;:&#34;5&#34;,&#34;label&#34;:&#34;Alpha&#34;}]"`,
		`data-no-results-label="No results"`,
		`data-clear-label="Clear"`,
		`for="org-parent-search-input"`,
		`type="text"`,
		`x-model="query"`,
		`type="hidden" name="parent_org_id"`,
		`:value="value"`,
		`id="organisation-search-results"`,
	}
	for _, r := range required {
		if !strings.Contains(html, r) {
			t.Errorf("rendered HTML missing %q:\n%s", r, html)
		}
	}
}

func TestSearchableSelectStaticMode(t *testing.T) {
	ctx := context.Background()
	props := SearchableSelectProps{
		ID:            "static-search",
		Name:          "choice",
		Label:         "Choice",
		StaticOptions: []SearchableSelectOption{{ID: "1", Label: "One"}, {ID: "2", Label: "Two"}},
	}

	html := renderSearchableSelectToString(t, ctx, props)

	if !strings.Contains(html, `data-search-action=""`) {
		t.Errorf("expected empty search action for static mode, got:\n%s", html)
	}
	if !strings.Contains(html, `[{&#34;id&#34;:&#34;1&#34;,&#34;label&#34;:&#34;One&#34;},{&#34;id&#34;:&#34;2&#34;,&#34;label&#34;:&#34;Two&#34;}]`) {
		t.Errorf("expected static options JSON, got:\n%s", html)
	}
}

func TestSearchableSelectMultiSelectRendersChips(t *testing.T) {
	ctx := context.Background()
	props := SearchableSelectProps{
		ID:          "multi-search",
		Name:        "tags",
		Label:       "Tags",
		MultiSelect: true,
		Selected:    []SearchableSelectOption{{ID: "1", Label: "One"}, {ID: "2", Label: "Two"}},
	}

	html := renderSearchableSelectToString(t, ctx, props)

	if !strings.Contains(html, `data-multiselect="true"`) {
		t.Errorf("expected multiselect data attribute, got:\n%s", html)
	}
	if !strings.Contains(html, `x-for="item in selected"`) {
		t.Errorf("expected chip template, got:\n%s", html)
	}
	if !strings.Contains(html, `@click.prevent="removeItem(item.id)"`) {
		t.Errorf("expected remove chip handler, got:\n%s", html)
	}
}

func TestSearchableSelectEscapesLabels(t *testing.T) {
	ctx := context.Background()
	props := SearchableSelectProps{
		ID:         "escape-search",
		Name:       "x",
		Label:      "Label <script>",
		ClearLabel: "Clear & Co",
		Selected:   []SearchableSelectOption{{ID: "1", Label: "A \"quoted\" name"}},
	}

	html := renderSearchableSelectToString(t, ctx, props)

	if strings.Contains(html, "<script>") {
		t.Errorf("rendered HTML must not contain a raw script tag, got:\n%s", html)
	}
	if !strings.Contains(html, "Clear &amp; Co") {
		t.Errorf("expected escaped ampersand in clear label, got:\n%s", html)
	}
	if !strings.Contains(html, `&#34;label&#34;:&#34;A \&#34;quoted\&#34; name&#34;`) {
		t.Errorf("expected escaped quotes in selected JSON, got:\n%s", html)
	}
}

func TestSearchableSelectIsCspSafe(t *testing.T) {
	ctx := context.Background()
	props := SearchableSelectProps{
		ID:           "csp-search",
		Name:         "x",
		Label:        "X",
		SearchAction: "search",
	}

	html := renderSearchableSelectToString(t, ctx, props)

	forbidden := []string{"new CustomEvent", "new ", "{ title:", "{title:", "{ message:", "{message:", "{ variant:", "{variant:"}
	for _, f := range forbidden {
		if strings.Contains(html, f) {
			t.Errorf("rendered HTML contains forbidden inline expression %q:\n%s", f, html)
		}
	}
}
