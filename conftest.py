collect_ignore = [
    # Standalone stress script; run it with: python test_integrations_stress.py
    # Not a pytest suite; functions take a live ControlPlane instance, not fixtures.
    "sdk/python/test_integrations_stress.py",
]
