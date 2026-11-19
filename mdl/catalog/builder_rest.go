// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// buildRestClients populates the rest_clients and rest_operations catalog tables.
func (b *Builder) buildRestClients() error {
	services, err := b.lister().ListConsumedRestServices()
	if err != nil {
		return err
	}

	svcStmt, err := b.tx.Prepare(`
		INSERT INTO rest_clients_data (Id, Name, QualifiedName, ModuleName, Folder,
			BaseUrl, BaseUrlConstant, AuthScheme, OperationCount, Documentation,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer svcStmt.Close()

	opStmt, err := b.tx.Prepare(`
		INSERT INTO rest_operations_data (Id, ServiceId, ServiceQualifiedName, Name,
			HttpMethod, Path, ParameterCount, HasBody, ResponseType, Timeout,
			ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer opStmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	opCount := 0
	for _, svc := range services {
		if !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + svc.Name

		authScheme := "None"
		if svc.Authentication != nil && svc.Authentication.Scheme != "" {
			authScheme = svc.Authentication.Scheme
		}

		_, err := svcStmt.Exec(
			string(svc.ID),
			svc.Name,
			qualifiedName,
			moduleName,
			b.hierarchy.buildFolderPath(svc.ContainerID), // real folder path (Bug 12b class)
			svc.BaseUrl,
			svc.BaseUrlConstant,
			authScheme,
			len(svc.Operations),
			svc.Documentation,
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}

		// Insert operations
		for _, op := range svc.Operations {
			hasBody := 0
			if op.BodyType != "" && op.BodyType != "NONE" {
				hasBody = 1
			}
			paramCount := len(op.Parameters) + len(op.QueryParameters)

			// Generate a synthetic ID for the operation
			opID := fmt.Sprintf("%x", sha256.Sum256([]byte(qualifiedName+"."+op.Name)))[:32]

			_, err := opStmt.Exec(
				opID,
				string(svc.ID),
				qualifiedName,
				op.Name,
				op.HttpMethod,
				op.Path,
				paramCount,
				hasBody,
				op.ResponseType,
				op.Timeout,
				moduleName,
				projectID, snapshotID,
			)
			if err != nil {
				return err
			}
			opCount++
		}
	}
	b.disown()

	b.report("REST Clients", len(services))
	if opCount > 0 {
		b.report("REST Operations", opCount)
	}
	return nil
}

// publishedRestRef is one published REST operation → microflow edge. sourceID is
// the operation's synthetic catalog id, so the emitted edge joins back to
// published_rest_operations_data rather than only naming the operation in prose.
type publishedRestRef struct{ owner, sourceID, qualifiedName, moduleName, microflow string }

// publishedRestOpName renders the name a published operation is known by in the
// reference graph. The empty parts are dropped rather than left as runs of
// spaces: an operation on a resource's own root has no path, and a name ending
// in whitespace is one a user cannot retype.
func publishedRestOpName(service, resource, method, path string) string {
	parts := make([]string, 0, 4)
	for _, p := range []string{service, resource, method, path} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " ")
}

// buildPublishedRestServices populates the published_rest_services and published_rest_operations catalog tables.
func (b *Builder) buildPublishedRestServices() error {
	services, err := b.lister().ListPublishedRestServices()
	if err != nil {
		return err
	}

	svcStmt, err := b.tx.Prepare(`
		INSERT INTO published_rest_services_data (Id, Name, QualifiedName, ModuleName, Folder,
			Path, Version, ServiceName, ResourceCount, OperationCount, Documentation,
			ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer svcStmt.Close()

	opStmt, err := b.tx.Prepare(`
		INSERT INTO published_rest_operations_data (Id, ServiceId, ServiceQualifiedName,
			ResourceName, HttpMethod, Path, Summary, Microflow, Deprecated,
			ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer opStmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	totalOps := 0
	for _, svc := range services {
		if !b.own(string(svc.ID)) {
			continue
		}
		moduleID := b.hierarchy.findModuleID(svc.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)
		qualifiedName := moduleName + "." + svc.Name

		// The custom authentication microflow runs on every authenticated
		// request; the operation edges below do not cover it (ako/mxcli#1639).
		b.addServiceMicroflowRefs(RefObjectPublishedRestService, svc.ID, qualifiedName, moduleName,
			RefKindPublish, svc.AuthenticationMicroflow)

		// Count total operations across all resources
		opCount := 0
		for _, res := range svc.Resources {
			opCount += len(res.Operations)
		}

		_, err := svcStmt.Exec(
			string(svc.ID),
			svc.Name,
			qualifiedName,
			moduleName,
			b.hierarchy.buildFolderPath(svc.ContainerID), // real folder path (Bug 12b class)
			svc.Path,
			svc.Version,
			svc.ServiceName,
			len(svc.Resources),
			opCount,
			svc.Documentation, // was always "", so QUAL002 could never close (mendixlabs/mxcli#1510)
			projectID, snapshotID,
		)
		if err != nil {
			return err
		}

		// Insert operations
		for _, res := range svc.Resources {
			for _, op := range res.Operations {
				deprecated := 0
				if op.Deprecated {
					deprecated = 1
				}

				opID := fmt.Sprintf("%x", sha256.Sum256([]byte(qualifiedName+"."+res.Name+"."+op.HTTPMethod+"."+op.Path)))[:32]

				_, err := opStmt.Exec(
					opID,
					string(svc.ID),
					qualifiedName,
					res.Name,
					op.HTTPMethod,
					op.Path,
					op.Summary,
					op.Microflow,
					deprecated,
					moduleName,
					projectID, snapshotID,
				)
				if err != nil {
					return err
				}
				totalOps++

				// An operation with no microflow is a real shape — Mendix allows
				// one while the service is being built — and an edge to the empty
				// name would collide with every other unnamed target in refs.
				if op.Microflow != "" {
					b.publishedRestRefs = append(b.publishedRestRefs, publishedRestRef{
						owner:    b.currentOwner(),
						sourceID: opID,
						// The resource is part of what makes an operation unique:
						// two resources of one service can both expose GET on the
						// same relative path.
						qualifiedName: publishedRestOpName(qualifiedName, res.Name, op.HTTPMethod, op.Path),
						moduleName:    moduleName,
						microflow:     op.Microflow,
					})
				}
			}
		}
	}
	b.disown()

	b.report("Published REST Services", len(services))
	if totalOps > 0 {
		b.report("Published REST Operations", totalOps)
	}
	return nil
}
