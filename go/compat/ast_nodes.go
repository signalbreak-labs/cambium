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

func (m *ASTModule) Kind() string {
	if m != nil && m.BelongsTo != nil {
		return "submodule"
	}
	return "module"
}
func (m *ASTModule) ParentNode() Node      { return nodeParent(m.Parent) }
func (m *ASTModule) NName() string         { return nodeName(m) }
func (m *ASTModule) Statement() *Statement { return nodeStatement(m.Source) }
func (m *ASTModule) Exts() []*Statement    { return nodeExtensions(m.Extensions) }
func (m *ASTModule) Groupings() []*Grouping {
	if m == nil {
		return nil
	}
	return m.Grouping
}
func (m *ASTModule) Typedefs() []*Typedef {
	if m == nil {
		return nil
	}
	return m.Typedef
}
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

func (Import) Kind() string             { return "import" }
func (s *Import) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Import) NName() string         { return nodeName(s) }
func (s *Import) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Import) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

type Include struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	RevisionDate *Value `yang:"revision-date"`
	Module       *Module
}

func (Include) Kind() string             { return "include" }
func (s *Include) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Include) NName() string         { return nodeName(s) }
func (s *Include) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Include) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

type Revision struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Description *Value `yang:"description"`
	Reference   *Value `yang:"reference"`
}

func (Revision) Kind() string             { return "revision" }
func (s *Revision) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Revision) NName() string         { return nodeName(s) }
func (s *Revision) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Revision) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

type BelongsTo struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	Prefix *Value `yang:"prefix,required"`
}

func (BelongsTo) Kind() string             { return "belongs-to" }
func (s *BelongsTo) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *BelongsTo) NName() string         { return nodeName(s) }
func (s *BelongsTo) Statement() *Statement { return nodeStatement(s.Source) }
func (s *BelongsTo) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Typedef) Kind() string             { return "typedef" }
func (s *Typedef) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Typedef) NName() string         { return nodeName(s) }
func (s *Typedef) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Typedef) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Type) Kind() string             { return "type" }
func (s *Type) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Type) NName() string         { return nodeName(s) }
func (s *Type) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Type) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Container) Kind() string              { return "container" }
func (s *Container) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *Container) NName() string          { return nodeName(s) }
func (s *Container) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *Container) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *Container) Groupings() []*Grouping { return s.Grouping }
func (s *Container) Typedefs() []*Typedef   { return s.Typedef }

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

func (Must) Kind() string             { return "must" }
func (s *Must) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Must) NName() string         { return nodeName(s) }
func (s *Must) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Must) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Leaf) Kind() string             { return "leaf" }
func (s *Leaf) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Leaf) NName() string         { return nodeName(s) }
func (s *Leaf) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Leaf) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (LeafList) Kind() string             { return "leaf-list" }
func (s *LeafList) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *LeafList) NName() string         { return nodeName(s) }
func (s *LeafList) Statement() *Statement { return nodeStatement(s.Source) }
func (s *LeafList) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (List) Kind() string              { return "list" }
func (s *List) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *List) NName() string          { return nodeName(s) }
func (s *List) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *List) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *List) Groupings() []*Grouping { return s.Grouping }
func (s *List) Typedefs() []*Typedef   { return s.Typedef }

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

func (Choice) Kind() string             { return "choice" }
func (s *Choice) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Choice) NName() string         { return nodeName(s) }
func (s *Choice) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Choice) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Case) Kind() string             { return "case" }
func (s *Case) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Case) NName() string         { return nodeName(s) }
func (s *Case) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Case) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (AnyXML) Kind() string             { return "anyxml" }
func (s *AnyXML) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *AnyXML) NName() string         { return nodeName(s) }
func (s *AnyXML) Statement() *Statement { return nodeStatement(s.Source) }
func (s *AnyXML) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (AnyData) Kind() string             { return "anydata" }
func (s *AnyData) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *AnyData) NName() string         { return nodeName(s) }
func (s *AnyData) Statement() *Statement { return nodeStatement(s.Source) }
func (s *AnyData) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Grouping) Kind() string              { return "grouping" }
func (s *Grouping) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *Grouping) NName() string          { return nodeName(s) }
func (s *Grouping) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *Grouping) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *Grouping) Groupings() []*Grouping { return s.Grouping }
func (s *Grouping) Typedefs() []*Typedef   { return s.Typedef }

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

func (Uses) Kind() string             { return "uses" }
func (s *Uses) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Uses) NName() string         { return nodeName(s) }
func (s *Uses) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Uses) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Refine) Kind() string             { return "refine" }
func (s *Refine) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Refine) NName() string         { return nodeName(s) }
func (s *Refine) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Refine) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (RPC) Kind() string              { return "rpc" }
func (s *RPC) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *RPC) NName() string          { return nodeName(s) }
func (s *RPC) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *RPC) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *RPC) Groupings() []*Grouping { return s.Grouping }
func (s *RPC) Typedefs() []*Typedef   { return s.Typedef }

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

func (Input) Kind() string              { return "input" }
func (s *Input) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *Input) NName() string          { return nodeName(s) }
func (s *Input) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *Input) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *Input) Groupings() []*Grouping { return s.Grouping }
func (s *Input) Typedefs() []*Typedef   { return s.Typedef }

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

func (Output) Kind() string              { return "output" }
func (s *Output) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *Output) NName() string          { return nodeName(s) }
func (s *Output) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *Output) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *Output) Groupings() []*Grouping { return s.Grouping }
func (s *Output) Typedefs() []*Typedef   { return s.Typedef }

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

func (Notification) Kind() string              { return "notification" }
func (s *Notification) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *Notification) NName() string          { return nodeName(s) }
func (s *Notification) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *Notification) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *Notification) Groupings() []*Grouping { return s.Grouping }
func (s *Notification) Typedefs() []*Typedef   { return s.Typedef }

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

func (Augment) Kind() string             { return "augment" }
func (s *Augment) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Augment) NName() string         { return nodeName(s) }
func (s *Augment) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Augment) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Extension) Kind() string             { return "extension" }
func (s *Extension) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Extension) NName() string         { return nodeName(s) }
func (s *Extension) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Extension) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

type Argument struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge" json:"-"`
	Parent     Node         `yang:"Parent,nomerge" json:"-"`
	Extensions []*Statement `yang:"Ext" json:",omitempty"`

	YinElement *Value `yang:"yin-element" json:",omitempty"`
}

func (Argument) Kind() string             { return "argument" }
func (s *Argument) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Argument) NName() string         { return nodeName(s) }
func (s *Argument) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Argument) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

type Element struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	YinElement *Value `yang:"yin-element"`
}

func (Element) Kind() string             { return "element" }
func (s *Element) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Element) NName() string         { return nodeName(s) }
func (s *Element) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Element) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Feature) Kind() string             { return "feature" }
func (s *Feature) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Feature) NName() string         { return nodeName(s) }
func (s *Feature) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Feature) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

type Deviation struct {
	Name       string       `yang:"Name,nomerge"`
	Source     *Statement   `yang:"Statement,nomerge"`
	Parent     Node         `yang:"Parent,nomerge"`
	Extensions []*Statement `yang:"Ext"`

	Description *Value     `yang:"description"`
	Deviate     []*Deviate `yang:"deviate,required"`
	Reference   *Value     `yang:"reference"`
}

func (Deviation) Kind() string             { return "deviation" }
func (s *Deviation) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Deviation) NName() string         { return nodeName(s) }
func (s *Deviation) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Deviation) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Deviate) Kind() string             { return "deviate" }
func (s *Deviate) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Deviate) NName() string         { return nodeName(s) }
func (s *Deviate) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Deviate) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Enum) Kind() string             { return "enum" }
func (s *Enum) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Enum) NName() string         { return nodeName(s) }
func (s *Enum) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Enum) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Bit) Kind() string             { return "bit" }
func (s *Bit) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Bit) NName() string         { return nodeName(s) }
func (s *Bit) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Bit) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Range) Kind() string             { return "range" }
func (s *Range) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Range) NName() string         { return nodeName(s) }
func (s *Range) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Range) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Length) Kind() string             { return "length" }
func (s *Length) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Length) NName() string         { return nodeName(s) }
func (s *Length) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Length) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Pattern) Kind() string             { return "pattern" }
func (s *Pattern) ParentNode() Node      { return nodeParent(s.Parent) }
func (s *Pattern) NName() string         { return nodeName(s) }
func (s *Pattern) Statement() *Statement { return nodeStatement(s.Source) }
func (s *Pattern) Exts() []*Statement    { return nodeExtensions(s.Extensions) }

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

func (Action) Kind() string              { return "action" }
func (s *Action) ParentNode() Node       { return nodeParent(s.Parent) }
func (s *Action) NName() string          { return nodeName(s) }
func (s *Action) Statement() *Statement  { return nodeStatement(s.Source) }
func (s *Action) Exts() []*Statement     { return nodeExtensions(s.Extensions) }
func (s *Action) Groupings() []*Grouping { return s.Grouping }
func (s *Action) Typedefs() []*Typedef   { return s.Typedef }

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
