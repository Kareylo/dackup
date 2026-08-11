package backend

// AvailableBackends lists the names of concrete backends that can be
// selected via the "dackup backend" command, in addition to the implicit
// default (empty Backend field -> default.Backend). Adding a backend means
// registering its name here plus adding one case each to ParseSettings and
// Factory.GetBackend.
func AvailableBackends() []string {
	return nil
}
