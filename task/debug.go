package task

import (
	"fmt"

	"github.com/rs/zerolog/log"
	gperr "github.com/yusing/goutils/errs"
)

type stuckSubtree struct {
	callbacks []string
	children  []string
}

// collect appends every pending callback and unfinished child in the subtree
// rooted at t. One traversal fills both lists, so their counts describe the same
// instant. Names are fully qualified, so a report says which route, provider or
// server is holding shutdown up.
func (s *stuckSubtree) collect(t *Task) {
	if t.callbacks != nil {
		for cb := range t.callbacks.Range {
			s.callbacks = append(s.callbacks, t.String()+": "+cb.about)
		}
	}
	if t.children != nil {
		for child := range t.children.Range {
			s.children = append(s.children, child.String())
			s.collect(child)
		}
	}
}

func (t *Task) reportStucked(cause error) {
	var stuck stuckSubtree
	stuck.collect(t)
	if len(stuck.callbacks) == 0 && len(stuck.children) == 0 {
		return
	}
	fmtOutput := gperr.NewBuilder(fmt.Sprintf("%s stucked callbacks: %d, stucked children: %d (%s)", t.String(), len(stuck.callbacks), len(stuck.children), cause))
	if len(stuck.callbacks) > 0 {
		callbackBuilder := gperr.NewBuilder("callbacks")
		for _, c := range stuck.callbacks {
			callbackBuilder.Adds(c)
		}
		fmtOutput.Add(callbackBuilder.Error())
	}
	if len(stuck.children) > 0 {
		childrenBuilder := gperr.NewBuilder("children")
		for _, c := range stuck.children {
			childrenBuilder.Adds(c)
		}
		fmtOutput.Add(childrenBuilder.Error())
	}
	log.Warn().Msg(fmtOutput.String())
}
