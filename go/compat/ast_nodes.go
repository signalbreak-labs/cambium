// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 signalbreak-labs

package compat

// ASTModule is the parser-facing module/submodule node used by compatibility
// helpers that accept in-memory ASTs.
type ASTModule struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext"`

	Anydata      []*AnyData      `yang:"anydata"`
	Anyxml       []*AnyXML       `yang:"anyxml"`
	Augment      []*Augment      `yang:"augment"`
	BelongsTo    *BelongsTo      `yang:"belongs-to,required=submodule,nomerge"`
	Choice       []*Choice       `yang:"choice"`
	Contact      *Value          `yang:"contact,nomerge"`
	Container    []*Container    `yang:"container"`
	Description  *Value          `yang:"description,nomerge"`
	Deviation    []*Deviation    `yang:"deviation"`
	Extension    []*Extension    `yang:"extension"`
	Feature      []*Feature      `yang:"feature"`
	Grouping     []*Grouping     `yang:"grouping"`
	Identity     []*ASTIdentity  `yang:"identity"`
	Import       []*Import       `yang:"import"`
	Include      []*Include      `yang:"include"`
	Leaf         []*Leaf         `yang:"leaf"`
	LeafList     []*LeafList     `yang:"leaf-list"`
	List         []*List         `yang:"list"`
	Namespace    *Value          `yang:"namespace,required=module,nomerge"`
	Notification []*Notification `yang:"notification"`
	Organization *Value          `yang:"organization,nomerge"`
	Prefix       *Value          `yang:"prefix,required=module,nomerge"`
	Reference    *Value          `yang:"reference,nomerge"`
	Revision     []*Revision     `yang:"revision,nomerge"`
	RPC          []*RPC          `yang:"rpc"`
	Typedef      []*Typedef      `yang:"typedef"`
	Uses         []*Uses         `yang:"uses"`
	YangVersion  *Value          `yang:"yang-version,nomerge"`
	Modules      *Modules
}

// Kind reports "submodule" when the node belongs to a module, otherwise "module".
func (m *ASTModule) Kind() string {
	if m != nil && m.BelongsTo != nil {
		return "submodule"
	}
	return "module"
}

// ParentNode returns the enclosing AST node, or nil at the root.
func (m *ASTModule) ParentNode() Node { return nodeParent(m.Parent) }

// NName returns the module name.
func (m *ASTModule) NName() string { return nodeName(m) }

// Statement returns the source statement the module was parsed from.
func (m *ASTModule) Statement() *Statement { return nodeStatement(m.Source) }

// Exts returns the module's extension statements.
func (m *ASTModule) Exts() []*Statement { return nodeExtensions(m.Extensions) }

// Groupings returns the module's top-level groupings in declaration order.
func (m *ASTModule) Groupings() []*Grouping {
	if m == nil {
		return nil
	}
	return m.Grouping
}

// Typedefs returns the module's top-level typedefs in declaration order.
func (m *ASTModule) Typedefs() []*Typedef {
	if m == nil {
		return nil
	}
	return m.Typedef
}

// GetPrefix returns the module's prefix, or the belongs-to prefix for a submodule.
func (m *ASTModule) GetPrefix() string {
	if m == nil {
		return ""
	}
	if m.BelongsTo != nil {
		if m.BelongsTo.Prefix != nil {
			return m.BelongsTo.Prefix.Name
		}
		return ""
	}
	if m.Prefix == nil {
		return ""
	}
	return m.Prefix.Name
}

// Import is the parser-facing import statement node.
type Import struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext"`

	Prefix       *Value `yang:"prefix,required"`
	RevisionDate *Value `yang:"revision-date"`
	Reference    *Value `yang:"reference,nomerge"`
	Description  *Value `yang:"description,nomerge"`
	Module       *Module
}

// Kind reports the node kind, always "import".
func (Import) Kind() string { return "import" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Import) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the imported module name.
func (s *Import) NName() string { return nodeName(s) }

// Statement returns the source statement the import was parsed from.
func (s *Import) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the import's extension statements.
func (s *Import) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Include is the parser-facing include statement node.
type Include struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	RevisionDate *Value `yang:"revision-date"`
	Module       *Module
}

// Kind reports the node kind, always "include".
func (Include) Kind() string { return "include" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Include) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Include) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Include) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Include) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Revision is the parser-facing revision statement node.
type Revision struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Description *Value `yang:"description"`
	Reference   *Value `yang:"reference"`
}

// Kind reports the node kind, always "revision".
func (Revision) Kind() string { return "revision" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Revision) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Revision) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Revision) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Revision) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// BelongsTo is the parser-facing belongs-to statement node.
type BelongsTo struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Prefix *Value `yang:"prefix,required"`
}

// Kind reports the node kind, always "belongs-to".
func (BelongsTo) Kind() string { return "belongs-to" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *BelongsTo) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *BelongsTo) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *BelongsTo) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *BelongsTo) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Typedef is the parser-facing typedef statement node.
type Typedef struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Default     *Value    `yang:"default"`
	Description *Value    `yang:"description"`
	Reference   *Value    `yang:"reference"`
	Status      *Value    `yang:"status"`
	Type        *Type     `yang:"type,required"`
	Units       *Value    `yang:"units"`
	YangType    *YangType `json:"-"`
}

// Kind reports the node kind, always "typedef".
func (Typedef) Kind() string { return "typedef" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Typedef) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Typedef) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Typedef) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Typedef) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Type is the parser-facing type statement node.
type Type struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	IdentityBase    *Value     `yang:"base"`
	Bit             []*Bit     `yang:"bit"`
	Enum            []*Enum    `yang:"enum"`
	FractionDigits  *Value     `yang:"fraction-digits"`
	Length          *Length    `yang:"length"`
	Path            *Value     `yang:"path"`
	Pattern         []*Pattern `yang:"pattern"`
	Range           *Range     `yang:"range"`
	RequireInstance *Value     `yang:"require-instance"`
	Type            []*Type    `yang:"type"`
	YangType        *YangType
}

// Kind reports the node kind, always "type".
func (Type) Kind() string { return "type" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Type) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Type) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Type) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Type) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Container is the parser-facing container statement node.
type Container struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata      []*AnyData      `yang:"anydata"`
	Action       []*Action       `yang:"action"`
	Anyxml       []*AnyXML       `yang:"anyxml"`
	Choice       []*Choice       `yang:"choice"`
	Config       *Value          `yang:"config"`
	Container    []*Container    `yang:"container"`
	Description  *Value          `yang:"description"`
	Grouping     []*Grouping     `yang:"grouping"`
	IfFeature    []*Value        `yang:"if-feature"`
	Leaf         []*Leaf         `yang:"leaf"`
	LeafList     []*LeafList     `yang:"leaf-list"`
	List         []*List         `yang:"list"`
	Must         []*Must         `yang:"must"`
	Notification []*Notification `yang:"notification"`
	Presence     *Value          `yang:"presence"`
	Reference    *Value          `yang:"reference"`
	Status       *Value          `yang:"status"`
	Typedef      []*Typedef      `yang:"typedef"`
	Uses         []*Uses         `yang:"uses"`
	When         *Value          `yang:"when"`
}

// Kind reports the node kind, always "container".
func (Container) Kind() string { return "container" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Container) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Container) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Container) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Container) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *Container) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *Container) Typedefs() []*Typedef { return s.Typedef }

// Must is the parser-facing must statement node.
type Must struct {
	Name       string       `yang:"Name,nomerge" json:",omitempty"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Description  *Value `yang:"description" json:",omitempty"`
	ErrorAppTag  *Value `yang:"error-app-tag" json:",omitempty"`
	ErrorMessage *Value `yang:"error-message" json:",omitempty"`
	Reference    *Value `yang:"reference" json:",omitempty"`
}

// Kind reports the node kind, always "must".
func (Must) Kind() string { return "must" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Must) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Must) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Must) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Must) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Leaf is the parser-facing leaf statement node.
type Leaf struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Config      *Value   `yang:"config"`
	Default     *Value   `yang:"default"`
	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	Mandatory   *Value   `yang:"mandatory"`
	Must        []*Must  `yang:"must"`
	Reference   *Value   `yang:"reference"`
	Status      *Value   `yang:"status"`
	Type        *Type    `yang:"type,required"`
	Units       *Value   `yang:"units"`
	When        *Value   `yang:"when"`
}

// Kind reports the node kind, always "leaf".
func (Leaf) Kind() string { return "leaf" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Leaf) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Leaf) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Leaf) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Leaf) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// LeafList is the parser-facing leaf-list statement node.
type LeafList struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Config      *Value   `yang:"config"`
	Default     []*Value `yang:"default"`
	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	MaxElements *Value   `yang:"max-elements"`
	MinElements *Value   `yang:"min-elements"`
	Must        []*Must  `yang:"must"`
	OrderedBy   *Value   `yang:"ordered-by"`
	Reference   *Value   `yang:"reference"`
	Status      *Value   `yang:"status"`
	Type        *Type    `yang:"type,required"`
	Units       *Value   `yang:"units"`
	When        *Value   `yang:"when"`
}

// Kind reports the node kind, always "leaf-list".
func (LeafList) Kind() string { return "leaf-list" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *LeafList) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *LeafList) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *LeafList) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *LeafList) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// List is the parser-facing list statement node.
type List struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata      []*AnyData      `yang:"anydata"`
	Action       []*Action       `yang:"action"`
	Anyxml       []*AnyXML       `yang:"anyxml"`
	Choice       []*Choice       `yang:"choice"`
	Config       *Value          `yang:"config"`
	Container    []*Container    `yang:"container"`
	Description  *Value          `yang:"description"`
	Grouping     []*Grouping     `yang:"grouping"`
	IfFeature    []*Value        `yang:"if-feature"`
	Key          *Value          `yang:"key"`
	Leaf         []*Leaf         `yang:"leaf"`
	LeafList     []*LeafList     `yang:"leaf-list"`
	List         []*List         `yang:"list"`
	MaxElements  *Value          `yang:"max-elements"`
	MinElements  *Value          `yang:"min-elements"`
	Must         []*Must         `yang:"must"`
	Notification []*Notification `yang:"notification"`
	OrderedBy    *Value          `yang:"ordered-by"`
	Reference    *Value          `yang:"reference"`
	Status       *Value          `yang:"status"`
	Typedef      []*Typedef      `yang:"typedef"`
	Unique       []*Value        `yang:"unique"`
	Uses         []*Uses         `yang:"uses"`
	When         *Value          `yang:"when"`
}

// Kind reports the node kind, always "list".
func (List) Kind() string { return "list" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *List) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *List) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *List) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *List) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *List) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *List) Typedefs() []*Typedef { return s.Typedef }

// Choice is the parser-facing choice statement node.
type Choice struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata     []*AnyData   `yang:"anydata"`
	Anyxml      []*AnyXML    `yang:"anyxml"`
	Case        []*Case      `yang:"case"`
	Config      *Value       `yang:"config"`
	Container   []*Container `yang:"container"`
	Default     *Value       `yang:"default"`
	Description *Value       `yang:"description"`
	IfFeature   []*Value     `yang:"if-feature"`
	Leaf        []*Leaf      `yang:"leaf"`
	LeafList    []*LeafList  `yang:"leaf-list"`
	List        []*List      `yang:"list"`
	Mandatory   *Value       `yang:"mandatory"`
	Reference   *Value       `yang:"reference"`
	Status      *Value       `yang:"status"`
	When        *Value       `yang:"when"`
}

// Kind reports the node kind, always "choice".
func (Choice) Kind() string { return "choice" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Choice) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Choice) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Choice) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Choice) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Case is the parser-facing case statement node.
type Case struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata     []*AnyData   `yang:"anydata"`
	Anyxml      []*AnyXML    `yang:"anyxml"`
	Choice      []*Choice    `yang:"choice"`
	Container   []*Container `yang:"container"`
	Description *Value       `yang:"description"`
	IfFeature   []*Value     `yang:"if-feature"`
	Leaf        []*Leaf      `yang:"leaf"`
	LeafList    []*LeafList  `yang:"leaf-list"`
	List        []*List      `yang:"list"`
	Reference   *Value       `yang:"reference"`
	Status      *Value       `yang:"status"`
	Uses        []*Uses      `yang:"uses"`
	When        *Value       `yang:"when"`
}

// Kind reports the node kind, always "case".
func (Case) Kind() string { return "case" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Case) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Case) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Case) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Case) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// AnyXML is the parser-facing anyxml statement node.
type AnyXML struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Config      *Value   `yang:"config"`
	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	Mandatory   *Value   `yang:"mandatory"`
	Must        []*Must  `yang:"must"`
	Reference   *Value   `yang:"reference"`
	Status      *Value   `yang:"status"`
	When        *Value   `yang:"when"`
}

// Kind reports the node kind, always "anyxml".
func (AnyXML) Kind() string { return "anyxml" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *AnyXML) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *AnyXML) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *AnyXML) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *AnyXML) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// AnyData is the parser-facing anydata statement node.
type AnyData struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Config      *Value   `yang:"config"`
	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	Mandatory   *Value   `yang:"mandatory"`
	Must        []*Must  `yang:"must"`
	Reference   *Value   `yang:"reference"`
	Status      *Value   `yang:"status"`
	When        *Value   `yang:"when"`
}

// Kind reports the node kind, always "anydata".
func (AnyData) Kind() string { return "anydata" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *AnyData) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *AnyData) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *AnyData) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *AnyData) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Grouping is the parser-facing grouping statement node.
type Grouping struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata      []*AnyData      `yang:"anydata"`
	Action       []*Action       `yang:"action"`
	Anyxml       []*AnyXML       `yang:"anyxml"`
	Choice       []*Choice       `yang:"choice"`
	Container    []*Container    `yang:"container"`
	Description  *Value          `yang:"description"`
	Grouping     []*Grouping     `yang:"grouping"`
	Leaf         []*Leaf         `yang:"leaf"`
	LeafList     []*LeafList     `yang:"leaf-list"`
	List         []*List         `yang:"list"`
	Notification []*Notification `yang:"notification"`
	Reference    *Value          `yang:"reference"`
	Status       *Value          `yang:"status"`
	Typedef      []*Typedef      `yang:"typedef"`
	Uses         []*Uses         `yang:"uses"`
}

// Kind reports the node kind, always "grouping".
func (Grouping) Kind() string { return "grouping" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Grouping) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Grouping) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Grouping) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Grouping) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *Grouping) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *Grouping) Typedefs() []*Typedef { return s.Typedef }

// Uses is the parser-facing uses statement node.
type Uses struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:"-"`

	Augment     *Augment  `yang:"augment" json:",omitempty"`
	Description *Value    `yang:"description" json:",omitempty"`
	IfFeature   []*Value  `yang:"if-feature" json:"-"`
	Refine      []*Refine `yang:"refine" json:"-"`
	Reference   *Value    `yang:"reference" json:"-"`
	Status      *Value    `yang:"status" json:"-"`
	When        *Value    `yang:"when" json:",omitempty"`
}

// Kind reports the node kind, always "uses".
func (Uses) Kind() string { return "uses" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Uses) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Uses) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Uses) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Uses) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Refine is the parser-facing refine statement node.
type Refine struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Default     *Value   `yang:"default"`
	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	Reference   *Value   `yang:"reference"`
	Config      *Value   `yang:"config"`
	Mandatory   *Value   `yang:"mandatory"`
	Presence    *Value   `yang:"presence"`
	Must        []*Must  `yang:"must"`
	MaxElements *Value   `yang:"max-elements"`
	MinElements *Value   `yang:"min-elements"`
}

// Kind reports the node kind, always "refine".
func (Refine) Kind() string { return "refine" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Refine) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Refine) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Refine) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Refine) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// RPC is the parser-facing rpc statement node.
type RPC struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description *Value      `yang:"description"`
	Grouping    []*Grouping `yang:"grouping"`
	IfFeature   []*Value    `yang:"if-feature"`
	Input       *Input      `yang:"input"`
	Output      *Output     `yang:"output"`
	Reference   *Value      `yang:"reference"`
	Status      *Value      `yang:"status"`
	Typedef     []*Typedef  `yang:"typedef"`
}

// Kind reports the node kind, always "rpc".
func (RPC) Kind() string { return "rpc" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *RPC) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *RPC) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *RPC) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *RPC) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *RPC) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *RPC) Typedefs() []*Typedef { return s.Typedef }

// Input is the parser-facing rpc/action input statement node.
type Input struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata   []*AnyData   `yang:"anydata"`
	Anyxml    []*AnyXML    `yang:"anyxml"`
	Choice    []*Choice    `yang:"choice"`
	Container []*Container `yang:"container"`
	Grouping  []*Grouping  `yang:"grouping"`
	Leaf      []*Leaf      `yang:"leaf"`
	LeafList  []*LeafList  `yang:"leaf-list"`
	List      []*List      `yang:"list"`
	Typedef   []*Typedef   `yang:"typedef"`
	Uses      []*Uses      `yang:"uses"`
}

// Kind reports the node kind, always "input".
func (Input) Kind() string { return "input" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Input) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Input) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Input) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Input) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *Input) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *Input) Typedefs() []*Typedef { return s.Typedef }

// Output is the parser-facing rpc/action output statement node.
type Output struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata   []*AnyData   `yang:"anydata"`
	Anyxml    []*AnyXML    `yang:"anyxml"`
	Choice    []*Choice    `yang:"choice"`
	Container []*Container `yang:"container"`
	Grouping  []*Grouping  `yang:"grouping"`
	Leaf      []*Leaf      `yang:"leaf"`
	LeafList  []*LeafList  `yang:"leaf-list"`
	List      []*List      `yang:"list"`
	Typedef   []*Typedef   `yang:"typedef"`
	Uses      []*Uses      `yang:"uses"`
}

// Kind reports the node kind, always "output".
func (Output) Kind() string { return "output" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Output) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Output) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Output) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Output) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *Output) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *Output) Typedefs() []*Typedef { return s.Typedef }

// Notification is the parser-facing notification statement node.
type Notification struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata     []*AnyData   `yang:"anydata"`
	Anyxml      []*AnyXML    `yang:"anyxml"`
	Choice      []*Choice    `yang:"choice"`
	Container   []*Container `yang:"container"`
	Description *Value       `yang:"description"`
	Grouping    []*Grouping  `yang:"grouping"`
	IfFeature   []*Value     `yang:"if-feature"`
	Leaf        []*Leaf      `yang:"leaf"`
	LeafList    []*LeafList  `yang:"leaf-list"`
	List        []*List      `yang:"list"`
	Reference   *Value       `yang:"reference"`
	Status      *Value       `yang:"status"`
	Typedef     []*Typedef   `yang:"typedef"`
	Uses        []*Uses      `yang:"uses"`
}

// Kind reports the node kind, always "notification".
func (Notification) Kind() string { return "notification" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Notification) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Notification) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Notification) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Notification) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *Notification) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *Notification) Typedefs() []*Typedef { return s.Typedef }

// Augment is the parser-facing augment statement node.
type Augment struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Anydata      []*AnyData      `yang:"anydata"`
	Action       []*Action       `yang:"action"`
	Anyxml       []*AnyXML       `yang:"anyxml"`
	Case         []*Case         `yang:"case"`
	Choice       []*Choice       `yang:"choice"`
	Container    []*Container    `yang:"container"`
	Description  *Value          `yang:"description"`
	IfFeature    []*Value        `yang:"if-feature"`
	Leaf         []*Leaf         `yang:"leaf"`
	LeafList     []*LeafList     `yang:"leaf-list"`
	List         []*List         `yang:"list"`
	Notification []*Notification `yang:"notification"`
	Reference    *Value          `yang:"reference"`
	Status       *Value          `yang:"status"`
	Uses         []*Uses         `yang:"uses"`
	When         *Value          `yang:"when"`
}

// Kind reports the node kind, always "augment".
func (Augment) Kind() string { return "augment" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Augment) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Augment) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Augment) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Augment) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Extension is the parser-facing extension statement node.
type Extension struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Argument    *Argument `yang:"argument" json:",omitempty"`
	Description *Value    `yang:"description" json:",omitempty"`
	Reference   *Value    `yang:"reference" json:",omitempty"`
	Status      *Value    `yang:"status" json:",omitempty"`
}

// Kind reports the node kind, always "extension".
func (Extension) Kind() string { return "extension" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Extension) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Extension) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Extension) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Extension) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Argument is the parser-facing extension-argument statement node.
type Argument struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	YinElement *Value `yang:"yin-element" json:",omitempty"`
}

// Kind reports the node kind, always "argument".
func (Argument) Kind() string { return "argument" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Argument) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Argument) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Argument) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Argument) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Element is the parser-facing yin-element statement node.
type Element struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	YinElement *Value `yang:"yin-element"`
}

// Kind reports the node kind, always "element".
func (Element) Kind() string { return "element" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Element) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Element) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Element) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Element) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Feature is the parser-facing feature statement node.
type Feature struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Description *Value   `yang:"description" json:",omitempty"`
	IfFeature   []*Value `yang:"if-feature" json:",omitempty"`
	Status      *Value   `yang:"status" json:",omitempty"`
	Reference   *Value   `yang:"reference" json:",omitempty"`
}

// Kind reports the node kind, always "feature".
func (Feature) Kind() string { return "feature" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Feature) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Feature) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Feature) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Feature) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Deviation is the parser-facing deviation statement node.
type Deviation struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description *Value     `yang:"description"`
	Deviate     []*Deviate `yang:"deviate,required"`
	Reference   *Value     `yang:"reference"`
}

// Kind reports the node kind, always "deviation".
func (Deviation) Kind() string { return "deviation" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Deviation) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Deviation) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Deviation) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Deviation) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Deviate is the parser-facing deviate statement node.
type Deviate struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Config      *Value   `yang:"config"`
	Default     *Value   `yang:"default"`
	Mandatory   *Value   `yang:"mandatory"`
	MaxElements *Value   `yang:"max-elements"`
	MinElements *Value   `yang:"min-elements"`
	Must        []*Must  `yang:"must"`
	Type        *Type    `yang:"type"`
	Unique      []*Value `yang:"unique"`
	Units       *Value   `yang:"units"`
}

// Kind reports the node kind, always "deviate".
func (Deviate) Kind() string { return "deviate" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Deviate) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Deviate) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Deviate) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Deviate) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Enum is the parser-facing enum statement node.
type Enum struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	Reference   *Value   `yang:"reference"`
	Status      *Value   `yang:"status"`
	Value       *Value   `yang:"value"`
}

// Kind reports the node kind, always "enum".
func (Enum) Kind() string { return "enum" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Enum) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Enum) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Enum) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Enum) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Bit is the parser-facing bit statement node.
type Bit struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description *Value   `yang:"description"`
	IfFeature   []*Value `yang:"if-feature"`
	Reference   *Value   `yang:"reference"`
	Status      *Value   `yang:"status"`
	Position    *Value   `yang:"position"`
}

// Kind reports the node kind, always "bit".
func (Bit) Kind() string { return "bit" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Bit) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Bit) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Bit) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Bit) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Range is the parser-facing range statement node.
type Range struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description  *Value `yang:"description"`
	ErrorAppTag  *Value `yang:"error-app-tag"`
	ErrorMessage *Value `yang:"error-message"`
	Reference    *Value `yang:"reference"`
}

// Kind reports the node kind, always "range".
func (Range) Kind() string { return "range" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Range) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Range) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Range) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Range) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Length is the parser-facing length statement node.
type Length struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description  *Value `yang:"description"`
	ErrorAppTag  *Value `yang:"error-app-tag"`
	ErrorMessage *Value `yang:"error-message"`
	Reference    *Value `yang:"reference"`
}

// Kind reports the node kind, always "length".
func (Length) Kind() string { return "length" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Length) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Length) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Length) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Length) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Pattern is the parser-facing pattern statement node.
type Pattern struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description  *Value `yang:"description"`
	ErrorAppTag  *Value `yang:"error-app-tag"`
	ErrorMessage *Value `yang:"error-message"`
	Reference    *Value `yang:"reference"`
	Modifier     *Value `yang:"modifier"`
}

// Kind reports the node kind, always "pattern".
func (Pattern) Kind() string { return "pattern" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Pattern) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Pattern) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Pattern) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Pattern) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Action is the parser-facing action statement node.
type Action struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description *Value      `yang:"description"`
	Grouping    []*Grouping `yang:"grouping"`
	IfFeature   []*Value    `yang:"if-feature"`
	Input       *Input      `yang:"input"`
	Output      *Output     `yang:"output"`
	Reference   *Value      `yang:"reference"`
	Status      *Value      `yang:"status"`
	Typedef     []*Typedef  `yang:"typedef"`
}

// Kind reports the node kind, always "action".
func (Action) Kind() string { return "action" }

// ParentNode returns the enclosing AST node, or nil at the root.
func (s *Action) ParentNode() Node { return nodeParent(s.Parent) }

// NName returns the node name.
func (s *Action) NName() string { return nodeName(s) }

// Statement returns the source statement the node was parsed from.
func (s *Action) Statement() *Statement { return nodeStatement(s.Source) }

// Exts returns the node's extension statements.
func (s *Action) Exts() []*Statement { return nodeExtensions(s.Extensions) }

// Groupings returns the node's nested groupings in declaration order.
func (s *Action) Groupings() []*Grouping { return s.Grouping }

// Typedefs returns the node's nested typedefs in declaration order.
func (s *Action) Typedefs() []*Typedef { return s.Typedef }

func nodeParent(parent Node) Node { return parent }

func nodeStatement(source *Statement) *Statement { return source }

func nodeExtensions(exts []*Statement) []*Statement { return exts }

func nodeName(node interface{ nodeName() string }) string {
	return node.nodeName()
}

func (m *ASTModule) nodeName() string {
	if m == nil {
		return ""
	}
	return m.Name
}
func (s *Import) nodeName() string       { return stringNodeName(s) }
func (s *Include) nodeName() string      { return stringNodeName(s) }
func (s *Revision) nodeName() string     { return stringNodeName(s) }
func (s *BelongsTo) nodeName() string    { return stringNodeName(s) }
func (s *Typedef) nodeName() string      { return stringNodeName(s) }
func (s *Type) nodeName() string         { return stringNodeName(s) }
func (s *Container) nodeName() string    { return stringNodeName(s) }
func (s *Must) nodeName() string         { return stringNodeName(s) }
func (s *Leaf) nodeName() string         { return stringNodeName(s) }
func (s *LeafList) nodeName() string     { return stringNodeName(s) }
func (s *List) nodeName() string         { return stringNodeName(s) }
func (s *Choice) nodeName() string       { return stringNodeName(s) }
func (s *Case) nodeName() string         { return stringNodeName(s) }
func (s *AnyXML) nodeName() string       { return stringNodeName(s) }
func (s *AnyData) nodeName() string      { return stringNodeName(s) }
func (s *Grouping) nodeName() string     { return stringNodeName(s) }
func (s *Uses) nodeName() string         { return stringNodeName(s) }
func (s *Refine) nodeName() string       { return stringNodeName(s) }
func (s *RPC) nodeName() string          { return stringNodeName(s) }
func (s *Input) nodeName() string        { return stringNodeName(s) }
func (s *Output) nodeName() string       { return stringNodeName(s) }
func (s *Notification) nodeName() string { return stringNodeName(s) }
func (s *Augment) nodeName() string      { return stringNodeName(s) }
func (s *Extension) nodeName() string    { return stringNodeName(s) }
func (s *Argument) nodeName() string     { return stringNodeName(s) }
func (s *Element) nodeName() string      { return stringNodeName(s) }
func (s *Feature) nodeName() string      { return stringNodeName(s) }
func (s *Deviation) nodeName() string    { return stringNodeName(s) }
func (s *Deviate) nodeName() string      { return stringNodeName(s) }
func (s *Enum) nodeName() string         { return stringNodeName(s) }
func (s *Bit) nodeName() string          { return stringNodeName(s) }
func (s *Range) nodeName() string        { return stringNodeName(s) }
func (s *Length) nodeName() string       { return stringNodeName(s) }
func (s *Pattern) nodeName() string      { return stringNodeName(s) }
func (s *Action) nodeName() string       { return stringNodeName(s) }

type hasNameField interface {
	nameField() string
}

func stringNodeName(node hasNameField) string {
	if node == nil {
		return ""
	}
	return node.nameField()
}

func (s *Import) nameField() string       { return s.Name }
func (s *Include) nameField() string      { return s.Name }
func (s *Revision) nameField() string     { return s.Name }
func (s *BelongsTo) nameField() string    { return s.Name }
func (s *Typedef) nameField() string      { return s.Name }
func (s *Type) nameField() string         { return s.Name }
func (s *Container) nameField() string    { return s.Name }
func (s *Must) nameField() string         { return s.Name }
func (s *Leaf) nameField() string         { return s.Name }
func (s *LeafList) nameField() string     { return s.Name }
func (s *List) nameField() string         { return s.Name }
func (s *Choice) nameField() string       { return s.Name }
func (s *Case) nameField() string         { return s.Name }
func (s *AnyXML) nameField() string       { return s.Name }
func (s *AnyData) nameField() string      { return s.Name }
func (s *Grouping) nameField() string     { return s.Name }
func (s *Uses) nameField() string         { return s.Name }
func (s *Refine) nameField() string       { return s.Name }
func (s *RPC) nameField() string          { return s.Name }
func (s *Input) nameField() string        { return s.Name }
func (s *Output) nameField() string       { return s.Name }
func (s *Notification) nameField() string { return s.Name }
func (s *Augment) nameField() string      { return s.Name }
func (s *Extension) nameField() string    { return s.Name }
func (s *Argument) nameField() string     { return s.Name }
func (s *Element) nameField() string      { return s.Name }
func (s *Feature) nameField() string      { return s.Name }
func (s *Deviation) nameField() string    { return s.Name }
func (s *Deviate) nameField() string      { return s.Name }
func (s *Enum) nameField() string         { return s.Name }
func (s *Bit) nameField() string          { return s.Name }
func (s *Range) nameField() string        { return s.Name }
func (s *Length) nameField() string       { return s.Name }
func (s *Pattern) nameField() string      { return s.Name }
func (s *Action) nameField() string       { return s.Name }
