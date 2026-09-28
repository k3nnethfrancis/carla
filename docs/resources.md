# Research context

Carla explores document-grown character development. These are references, not
claims of affiliation or faithful replication:

- [Computer #4 model card](https://huggingface.co/cosmicoptima/computer-4): describes
  an anthology grown from DeepSeek-V3-Base and Gunkel's paths taxonomy, and a later
  KTO correction. Carla has neither reproduced that training nor implemented KTO.
- [Ideonomy repository](https://github.com/XyraSinclair/ideonomy): associated method
  context and source material.
- [Patrick Gunkel / MIT](https://ideonomy.mit.edu/gunkel.html) and the
  [paths taxonomy](https://ideonomy.mit.edu/divisions/paths/tbl-pathdo.html): source
  references for document-seeded exploration. The Paths table is not bundled
  because redistribution permission has not been verified.
- [Moving Castles: Zero](https://movingcastles.world/posts/zero): a related character
  training approach. Carla's current pipeline does not implement Zero's memory
  system or claim equivalent training results.
- [Meditations, George Long translation](https://www.gutenberg.org/ebooks/15877)
  and [Tractatus, Ogden translation](https://www.gutenberg.org/ebooks/5740): optional
  seed sources. Consider source-author imitation and edition/territory rights
  before using or redistributing derived datasets.

A local smaller base model, a manually curated anthology, an automated selection-policy
model and raw alternating conversation templates are separate experimental
choices. Keep their effects distinct when reporting results. Preserve model
revision/quantization, seed extraction, exact prompt, sampling, selections and
rejected alternatives. Conversational appearance alone is not evidence of
weight-level character alignment or successful transfer to another model.
