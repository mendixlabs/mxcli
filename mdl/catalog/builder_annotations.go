// SPDX-License-Identifier: Apache-2.0

package catalog

// buildDomainModelAnnotations writes one row per domain model annotation
// (ako/mxcli#1300). A microflow's annotations are activities and reach a rule
// through activities_for(); a domain model's were in no table, so a rule
// checking annotation text skipped every domain model and reported it clean.
//
// Each row belongs to its domain model's unit, as the entity rows do, so an
// incremental build rewrites a module's annotations exactly when it rewrites
// that module's entities.
func (b *Builder) buildDomainModelAnnotations() error {
	domainModels, err := b.cachedDomainModels()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO annotations_data (Id, DomainModelId, ModuleName, Caption, X, Y, Width,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()
	count := 0
	for _, dm := range domainModels {
		if !b.own(string(dm.ID)) {
			continue
		}
		moduleName := b.hierarchy.getModuleName(b.hierarchy.findModuleID(dm.ContainerID))
		for _, a := range dm.Annotations {
			if _, err := stmt.Exec(
				string(a.ID), string(dm.ID), moduleName, a.Caption,
				a.Location.X, a.Location.Y, a.Width,
				projectID, snapshotID,
			); err != nil {
				return err
			}
			count++
		}
	}

	b.report("Domain model annotations", count)
	return nil
}
