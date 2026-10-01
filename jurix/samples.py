"""Contracts used by the in-app sample indexer. Original text, not a filing."""

SAMPLE_MSA = """\
MASTER SERVICES AGREEMENT

between Atlas Freight, Inc. ("Customer") and Harborline Logistics LLC ("Supplier").

Article 8 — Limitation of Liability

§8.1 Exclusion of Consequential Damages. Neither party shall be liable to the other for any indirect, incidental, special, or consequential damages, including loss of profits or goodwill, arising out of this Agreement, even if advised of the possibility of such damages.

§8.2 Cap on Liability. Except for a party's obligations under Section 10 (Confidentiality), Supplier's aggregate liability shall not exceed the fees paid in the three months preceding the claim. The foregoing limitation applies whether a claim arises in contract, tort, or otherwise.

Article 9 — Data Protection

§9.1 Customer Data. Supplier shall process Customer Data solely to perform the Services and shall maintain administrative, physical, and technical safeguards appropriate to the nature of that data.

§9.2 Notices. All notices under this Agreement shall be in writing and delivered to the addresses in Schedule 1.

Article 12 — Term and Renewal

§12.1 Renewal. This Agreement shall renew automatically for successive one-year terms unless either party gives ninety (90) days' written notice of non-renewal.

§12.2 Termination for Cause. Either party may terminate this Agreement on written notice if the other party materially breaches it and fails to cure within thirty (30) days of that notice.

Article 14 — General Provisions

§14.4 Governing Law. This Agreement shall be governed by and construed in accordance with the laws of the State of Delaware, without regard to conflict-of-laws principles. The parties submit to the exclusive jurisdiction of the state and federal courts located in Wilmington, Delaware.
"""

SAMPLE_DPA = """\
DATA PROCESSING ADDENDUM

between Northwind Holdings, Inc. ("Customer") and Kepler Clinic Systems LLC ("Processor").

§4.1 Sub-processors. Processor may engage the sub-processors listed in Annex B, and shall give Customer thirty (30) days' prior notice before adding one.

§5.2 Breach notice. Processor shall notify Customer within twenty-four (24) hours after becoming aware of a security incident affecting Customer Data, and shall include the nature of the incident and the categories of data involved.

§7.1 Term. This Addendum renews automatically for successive one-year terms unless either party gives thirty (30) days' written notice of non-renewal.

§8.4 Liability. Processor's aggregate liability under this Addendum shall not exceed the fees paid in the twelve months preceding the claim.

§9.3 Governing law. This Addendum is governed by the laws of the State of California, without regard to conflict-of-laws principles.
"""

SAMPLES = {
    "msa": {"doc_name": "Harborline_MSA_v3.txt", "text": SAMPLE_MSA, "matter": "MSA · Harborline"},
    "dpa": {"doc_name": "Northwind_DPA.txt", "text": SAMPLE_DPA, "matter": "DPA refresh"},
}
