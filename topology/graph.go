package topology

import (
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/multi"
)

// dotGraph wraps a multi.UndirectedGraph for DOT unmarshaling.
type dotGraph struct {
	*multi.UndirectedGraph
}

func newDotGraph() *dotGraph {
	return &dotGraph{UndirectedGraph: multi.NewUndirectedGraph()}
}

// NewLine returns a DOT-aware line.
func (g *dotGraph) NewLine(from, to graph.Node) graph.Line {
	e := g.UndirectedGraph.NewLine(from, to).(multi.Line)
	return &dotLine{Line: e}
}

// NewNode returns a DOT-aware node.
func (g *dotGraph) NewNode() graph.Node {
	return &dotNode{Node: g.UndirectedGraph.NewNode()}
}

// SetLine allows the DOT unmarshaler to add lines to the graph.
func (g *dotGraph) SetLine(e graph.Line) {
	g.UndirectedGraph.SetLine(e.(*dotLine))
}

type dotPortLabels struct {
	Port, Compass string
}

// dotLine is a DOT-aware unweighted line.
type dotLine struct {
	multi.Line

	FromPortLabels dotPortLabels
	ToPortLabels   dotPortLabels
	attrs          map[string]string
}

// SetAttribute sets an attribute of the receiver.
func (e *dotLine) SetAttribute(attr encoding.Attribute) error {
	if e.attrs == nil {
		e.attrs = make(map[string]string)
	}
	e.attrs[attr.Key] = attr.Value
	return nil
}

// Attributes returns the DOT attributes of the edge.
func (e *dotLine) Attributes() []encoding.Attribute {
	return toAttributeSlice(e.attrs)
}

func (e *dotLine) SetFromPort(port, compass string) error {
	e.FromPortLabels.Port = port
	e.FromPortLabels.Compass = compass
	return nil
}

func (e *dotLine) SetToPort(port, compass string) error {
	e.ToPortLabels.Port = port
	e.ToPortLabels.Compass = compass
	return nil
}

func (e *dotLine) FromPort() (port, compass string) {
	return e.FromPortLabels.Port, e.FromPortLabels.Compass
}

func (e *dotLine) ToPort() (port, compass string) {
	return e.ToPortLabels.Port, e.ToPortLabels.Compass
}

// dotNode is a DOT-aware node.
type dotNode struct {
	graph.Node
	dotID string
	attrs map[string]string
}

// SetDOTID sets the DOT ID of the dotNode.
func (n *dotNode) SetDOTID(id string) { n.dotID = id }

func (n *dotNode) String() string { return n.dotID }

// SetAttribute sets a DOT attribute.
func (n *dotNode) SetAttribute(attr encoding.Attribute) error {
	if n.attrs == nil {
		n.attrs = make(map[string]string)
	}
	n.attrs[attr.Key] = attr.Value
	return nil
}

// Attributes returns the DOT attributes of the node.
func (n *dotNode) Attributes() []encoding.Attribute {
	return toAttributeSlice(n.attrs)
}

func toAttributeSlice(m map[string]string) []encoding.Attribute {
	as := make([]encoding.Attribute, 0, len(m))
	for k, v := range m {
		as = append(as, encoding.Attribute{
			Key:   k,
			Value: v,
		})
	}
	return as
}
