import re

with open('runtime/modelrepo/openvino/client.go', 'r') as f:
    content = f.read()

pattern = r'''type cachedEmbedSession struct \{
	key  string
	sess EmbedSessionBackend
	mu   sync\.Mutex
\}

var embedSessionCache = struct \{
	sync\.Mutex
	m map\[string\]\*cachedEmbedSession
\}\{m: map\[string\]\*cachedEmbedSession\{\}\}

func acquireCachedEmbedSession\(modelPath, device string\) \(\*cachedEmbedSession, error\) \{
	key := modelPath \+ "\\x00" \+ device
	embedSessionCache\.Lock\(\)
	if cs, ok := embedSessionCache\.m\[key\]; ok \{
		embedSessionCache\.Unlock\(\)
		return cs, nil
	\}
	embedSessionCache\.Unlock\(\)

	s, err := newEmbedSession\(modelPath, device\)
	if err != nil \{
		return nil, err
	\}
	embedSessionCache\.Lock\(\)
	defer embedSessionCache\.Unlock\(\)
	if cs, ok := embedSessionCache\.m\[key\]; ok \{
		_ = s\.Close\(\)
		return cs, nil
	\}
	cs := &cachedEmbedSession\{key: key, sess: s\}
	embedSessionCache\.m\[key\] = cs
	return cs, nil
\}

func dropCachedEmbedSession\(cs \*cachedEmbedSession\) \{
	if cs == nil \{
		return
	\}
	embedSessionCache\.Lock\(\)
	if embedSessionCache\.m\[cs\.key\] == cs \{
		delete\(embedSessionCache\.m, cs\.key\)
	\}
	embedSessionCache\.Unlock\(\)
	_ = cs\.sess\.Close\(\)
\}

type embedClient struct \{
	modelPath string
	device    string
\}

func \(c \*embedClient\) Embed\(ctx context\.Context, prompt string\) \(\[\]float64, error\) \{
	cs, err := acquireCachedEmbedSession\(c\.modelPath, c\.device\)
	if err != nil \{
		return nil, err
	\}
	cs\.mu\.Lock\(\)
	defer cs\.mu\.Unlock\(\)

	f32, err := cs\.sess\.Embed\(ctx, prompt\)
	if err != nil \{
		if fatalSessionError\(err\) \{
			dropCachedEmbedSession\(cs\)
		\}
		return nil, err
	\}
	out := make\(\[\]float64, len\(f32\)\)
	for i, v := range f32 \{
		out\[i\] = float64\(v\)
	\}
	return out, nil
\}'''

replacement = '''type embedClient struct {
	modelPath string
	device    string
}

func (c *embedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	// The native OpenVINO embeddings backend is in modeld. For now, the client
	// returns unsupported. Once we transport embeddings, this will call modeldconn.
	return nil, NewUnsupportedFeatureError("embed client (not implemented over transport)")
}'''

new_content = re.sub(pattern, replacement, content)

if new_content == content:
    print("NO REPLACEMENT MADE")
else:
    with open('runtime/modelrepo/openvino/client.go', 'w') as f:
        f.write(new_content)
    print("PATCH APPLIED")
