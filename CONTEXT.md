# Carla

Carla develops character data from source documents through continuations, conversations and assessment.

## Language

**Behavior**: A named specification of something a judge can observe in text. It describes what to look for; it does not prescribe how to detect it or what to do about it.

**Judge**: The model and assessment configuration used to observe behaviors, including its prompt, call mode and detection rules.

**Policy**: A set of behaviors, judge configurations and rules for using their results. Monitoring can warn or stop; Selection chooses a path; Evals records acceptance and may mark passing data for training.

**Observation**: What a judge reports about a behavior. It remains distinct from whether that result is wanted.

**Expected outcome**: Whether a policy requires a behavior to be present or absent for acceptance.

**Run**: A recorded execution over frozen data and configuration, including observations, outcomes and incomplete results.
