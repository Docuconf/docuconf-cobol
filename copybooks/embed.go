// Package copybooks holds the docuconf runtime that docuconf-cobol
// generate inlines into every generated loader: DCRTWS.cpy (working
// storage) and DCRTPD.cpy (procedure division paragraphs).
package copybooks

import _ "embed"

// WorkingStorage is DCRTWS.cpy.
//
//go:embed DCRTWS.cpy
var WorkingStorage string

// Procedures is DCRTPD.cpy.
//
//go:embed DCRTPD.cpy
var Procedures string
