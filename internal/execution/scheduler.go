package execution

import (
	"github.com/Jayj1997/gum-workflows/internal/workflow"
)

// scheduler 维护 Ready Queue 与 Dependency Counter（设计计划 §26 的算法形态，
// 避免 every-loop 全量扫描）。串行版按入队顺序逐个出队；
// M7 并行化时将其替换为并发消费同一队列。
type scheduler struct {
	// remaining 是每个 Node 尚未完成的前驱（Data + Control Edge）数量。
	remaining map[string]int
	// successors 是去重后的后继列表。
	successors map[string][]string
	// ready 是就绪队列（FIFO，初始为全部源节点）。
	ready []string
}

func newScheduler(g workflow.Graph) *scheduler {
	s := &scheduler{
		remaining:  make(map[string]int, len(g.NodeIDs)),
		successors: make(map[string][]string, len(g.NodeIDs)),
	}
	for _, id := range g.NodeIDs {
		s.remaining[id] = len(g.Predecessors(id))
		s.successors[id] = g.Successors(id)
		if s.remaining[id] == 0 {
			s.ready = append(s.ready, id)
		}
	}
	return s
}

// next 出队下一个就绪 Node；队列为空时返回空串。
func (s *scheduler) next() string {
	if len(s.ready) == 0 {
		return ""
	}
	id := s.ready[0]
	s.ready = s.ready[1:]
	return id
}

// done 报告就绪队列是否已空（串行语义下即执行结束）。
func (s *scheduler) done() bool {
	return len(s.ready) == 0
}

// complete 标记 Node 完成，递减其后继的依赖计数；
// 新就绪的 Node 入队并返回（供调用方迁移状态）。
func (s *scheduler) complete(id string) []string {
	var newlyReady []string
	for _, succ := range s.successors[id] {
		s.remaining[succ]--
		if s.remaining[succ] == 0 {
			s.ready = append(s.ready, succ)
			newlyReady = append(newlyReady, succ)
		}
	}
	return newlyReady
}
