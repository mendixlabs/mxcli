// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/model"
)

// buildChangeDataCaptureServices fills change_data_capture_services and
// change_data_capture_sources, and records what each service EXPOSES.
//
// The tables answer two different questions, which is why there are two. The
// service row is the document — its app name, how many entities it tracks, and
// the baseline revision mxbuild compares against. A source row is one tracked
// entity, and it carries the **Kafka topic** that entity's changes land on,
// computed the way Studio Pro's editor computes it
// (cdc.<AppName>.<ExposedName>.<Major>.<space>). That topic is the one value a
// downstream consumer needs and the one the model does not store anywhere: it
// is derived from three properties that live in two different halves of the
// document.
//
// The `expose` edges matter more than the tables. A CDC service is an EXIT from
// the model: nothing in the app calls it, and an entity whose changes are
// published is load-bearing for a consumer that mxcli cannot see. Without the
// edges, `refs`, `impact` and `show callers` read a CDC-only entity as unused —
// and a rename or a drop of it silently moves or ends a Kafka topic (the major
// is a topic component, so downstream that is a new table with no history).
func (b *Builder) buildChangeDataCaptureServices() error {
	services, err := b.cachedChangeDataCaptureServices()
	if err != nil {
		return err
	}

	svcStmt, err := b.tx.Prepare(`
		INSERT INTO change_data_capture_services_data (Id, Name, QualifiedName, ModuleName,
			Documentation, ServiceName, AppName, Excluded, Revision, SourceCount, PropertyCount,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer svcStmt.Close()

	srcStmt, err := b.tx.Prepare(`
		INSERT INTO change_data_capture_sources_data (Id, ServiceId, ServiceQualifiedName,
			Entity, ExposedName, Version, Topic, AttributeCount, AssociationCount,
			ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer srcStmt.Close()

	refStmt, err := b.tx.Prepare(`
		INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName,
			RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer refStmt.Close()

	projectID, snapshotID := b.snapshotMeta()
	sources := 0

	for _, svc := range services {
		if svc == nil || !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + svc.Name

		excluded := 0
		if svc.Excluded {
			excluded = 1
		}
		revision := ""
		if svc.Baseline != nil {
			revision = svc.Baseline.Version.String()
		}

		properties := 0
		for _, src := range svc.Sources {
			properties += len(src.Properties)
		}

		if _, err := svcStmt.Exec(
			string(svc.ID), svc.Name, qualifiedName, moduleName,
			svc.Documentation, svc.ServiceName, svc.AppName, excluded, revision,
			len(svc.Sources), properties,
			projectID, snapshotID,
		); err != nil {
			return err
		}

		for _, src := range svc.Sources {
			if src == nil {
				continue
			}
			version := svc.SourceVersion(src)
			attrs, assocs := 0, 0
			for _, p := range src.Properties {
				if p.IsAssociation() {
					assocs++
				} else {
					attrs++
				}
			}
			if _, err := srcStmt.Exec(
				string(src.ID), string(svc.ID), qualifiedName,
				src.Entity, src.ExposedName, version.String(),
				svc.Topic(src), attrs, assocs,
				moduleName, projectID, snapshotID,
			); err != nil {
				return err
			}
			sources++

			// One edge per exposed element. The entity's is what makes a
			// CDC-only entity visible to `impact`; the member edges are what
			// make "is this attribute safe to drop?" answerable.
			emit := func(targetType, targetName string) {
				if targetName == "" {
					return
				}
				_, _ = refStmt.Exec(RefObjectChangeDataCaptureService, string(svc.ID), qualifiedName,
					targetType, "", targetName,
					RefKindExpose, moduleName, projectID, snapshotID)
			}
			emit(RefObjectEntity, src.Entity)
			for _, p := range src.Properties {
				if p == nil {
					continue
				}
				if p.IsAssociation() {
					emit(RefObjectAssociation, p.Association)
					continue
				}
				emit(RefObjectAttribute, p.Attribute)
			}
		}
	}
	b.disown()

	b.report("Change Data Capture Services", len(services))
	b.report("Change Data Capture Sources", sources)
	return nil
}

// cachedChangeDataCaptureServices lists the project's CDC services once.
func (b *Builder) cachedChangeDataCaptureServices() ([]*model.ChangeDataCaptureService, error) {
	if b.changeDataCaptureCache == nil {
		var err error
		b.changeDataCaptureCache, err = b.lister().ListChangeDataCaptureServices()
		if err != nil {
			return nil, err
		}
	}
	return b.changeDataCaptureCache, nil
}
