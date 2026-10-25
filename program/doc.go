// Package program is the authorized-lab control plane for English ARTEX.
//
// It does not run tools and it does not describe how to attack anything. It
// decides whether a target is inside a loaded scope file, fences text before
// that text is put back into a prompt, and keeps the engagement record: the
// graph, the run log, the fireteam, and the score. A seated fireteam is
// advisory unless the desk is marked strict. Strict mode refuses a finding
// until the operator confirms the team.
//
// A scope file uses the Vivarium range-scope shape. Default is deny. A deny
// network wins over an allow network. Names that are not listed are out.
// The operator vault and flag values are not accepted by this package.
// A mounted tree contributes paths and import edges to the planner. File
// bodies stay on disk. A patch under .artex-patches can be checked against
// that tree. The check does not apply the patch and does not open a pull request.
package program
