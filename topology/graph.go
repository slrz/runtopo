package topology

import (
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/simple"
)

// dotGraph wraps a simple.UndirectedGraph for DOT unmarshaling.
type dotGraph struct {
	*simple.UndirectedGraph
}

func newDotGraph() *dotGraph {
	return &dotGraph{UndirectedGraph: simple.NewUndirectedGraph()}
}

// NewEdge returns a DOT-aware edge.
func (g *dotGraph) NewEdge(from, to graph.Node) graph.Edge {
	e := g.UndirectedGraph.NewEdge(from, to).(simple.Edge)
	return &dotEdge{Edge: e}
}

// NewNode returns a DOT-aware node.
func (g *dotGraph) NewNode() graph.Node {
	return &dotNode{Node: g.UndirectedGraph.NewNode()}
}

// SetEdge allows the DOT unmarshaler to add edges to the graph.
func (g *dotGraph) SetEdge(e graph.Edge) {
	g.UndirectedGraph.SetEdge(e.(*dotEdge))
}

type dotPortLabels struct {
	Port, Compass string
}

// dotEdge is a DOT-aware weighted edge.
type dotEdge struct {
	simple.Edge

	FromPortLabels dotPortLabels
	ToPortLabels   dotPortLabels
	attrs          map[string]string
}

// SetAttribute sets an attribute of the receiver.
func (e *dotEdge) SetAttribute(attr encoding.Attribute) error {
	if e.attrs == nil {
		e.attrs = make(map[string]string)
	}
	e.attrs[attr.Key] = attr.Value
	return nil
}

// Attributes returns the DOT attributes of the edge.
func (e *dotEdge) Attributes() []encoding.Attribute {
	return toAttributeSlice(e.attrs)
}

func (e *dotEdge) SetFromPort(port, compass string) error {
	e.FromPortLabels.Port = port
	e.FromPortLabels.Compass = compass
	return nil
}

func (e *dotEdge) SetToPort(port, compass string) error {
	e.ToPortLabels.Port = port
	e.ToPortLabels.Compass = compass
	return nil
}

func (e *dotEdge) FromPort() (port, compass string) {
	return e.FromPortLabels.Port, e.FromPortLabels.Compass
}

func (e *dotEdge) ToPort() (port, compass string) {
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
