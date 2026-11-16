package settings

// addFirst puts a platform engine at the front of its category, so the machine's
// own service is asked before one that needs a key and a network.
func addFirst(list *[]Provider, p Provider) {
	*list = append([]Provider{p}, *list...)
}

// selectFirst keeps the checked list empty where a context has no default, so
// "nothing selected" — which plan reads as "everything enabled" — is not turned
// into "only this one service".
func selectFirst(c *Config, context, id string) {
	ids := c.Selections[context]
	if len(ids) == 0 {
		return
	}
	for _, have := range ids {
		if have == id {
			return
		}
	}
	c.Selections[context] = append([]string{id}, ids...)
}
