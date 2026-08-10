import { test, before, after } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import os from 'os';
import path from 'path';
import { X509Certificate } from 'crypto';
import { config } from '../lib/config.js';

const prevLetsencrypt = config.letsencryptPath;
let tmpRoot = '';

// CN=a.example.com, SAN=a.example.com + b.example.com
const COVERING_FULLCHAIN = `-----BEGIN CERTIFICATE-----
MIIDMzCCAhugAwIBAgIQFl7eH0Zn6KhJiqdWn8dv7zANBgkqhkiG9w0BAQsFADAYMRYwFAYDVQQD
DA1hLmV4YW1wbGUuY29tMB4XDTI2MDgxMDE5MTkzNVoXDTM2MDgxMDE5MjkzNVowGDEWMBQGA1UE
AwwNYS5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALBxe5lwQAO4
KtzpiSVD/WDqtpciQxkWsfjzjnAXK+tKgfDRhtQ3L3rgaAALkl9JpC9hKo9YgU0nQ1c1z+JP6Yae
XWEfiJ573SpJGho9LdcB9EDzvRnNhR4QQnkwSTHmiWTPg5r21u9rYhwJlfYg752KoYtx7rxN8QUV
+hCel7oA71TX6N+JpN+9740fkjVX3Hl6kLf6uYtLZ6uTYIhVqxOpG8ilAGX2B7drjRlYVaNfAgYG
QwW9K3ciBWM/4Zt7DyaxcuXrE3zRqkajCTZOvcKwTd7PUQwIFkoiNqqSZEXriBYchDfGZxN6PRax
aRNLYEJvWqoSfW5Mw/P8+xgnj7UCAwEAAaN5MHcwDgYDVR0PAQH/BAQDAgWgMB0GA1UdJQQWMBQG
CCsGAQUFBwMCBggrBgEFBQcDATAnBgNVHREEIDAegg1hLmV4YW1wbGUuY29tgg1iLmV4YW1wbGUu
Y29tMB0GA1UdDgQWBBQ0BqbAJA4K6kUtagv9PYCgBFjV2TANBgkqhkiG9w0BAQsFAAOCAQEAhWsW
ahQwGBPt50t8u6UiAp8idAs48wBFVTi11lDcdoj2VP8EBvitn66GyMsiRAFarnSmY+qha9d2RHOa
rAVT2+loVwKI891iAaqfDNcM7hwqbG8dRGqJhMiwhVJzcDvhU8emS7UZsNyclJIWl7PrVLltE6ro
YAKaQJEYJwx6+4W9KPGGY03kT9IJgaAO4PUqkyvTrEK0En6io7Tj6zUaxlPU7VjnF2A5OMB5pueQ
HuYlCl2TNCaq3J2z5EOkExwIuh/kFXRkyUafhndLcpTLi99rDFBfR4MQ25OIewhCLGBsQSNecocC
27A9kz2B0UvRJ6g6mifKcKKshVlYXEV8og==
-----END CERTIFICATE-----
`;

// Orphan: CN/SAN = b.example.com only (simulates leftover live/<new-primary>/)
const ORPHAN_B_FULLCHAIN = `-----BEGIN CERTIFICATE-----
MIIDJDCCAgygAwIBAgIQKusVyad3CYZCH3Lx2sQSUTANBgkqhkiG9w0BAQsFADAYMRYwFAYDVQQD
DA1iLmV4YW1wbGUuY29tMB4XDTI2MDgxMDIxMTEzOVoXDTM2MDgxMDIxMjEzOVowGDEWMBQGA1UE
AwwNYi5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAL0Iqwy1ANI3
wK/U/BJb1ey8sM1nivi9lZtUgc8GfCXb40CGOPcxOUtTsQQnWcmbEgC+2PLtFgTij4OhhtvOakkp
Qo2rLmkaXeiL3Lr+0Xa1T0yXNaUxtpMBYgcyaGHZNeqUyIGNioXLOFhDe2pnlI5N6VLo2RHs28+g
rOGM1jkCVltPFAF7G/Aq7+I64o2tzLwTyV3MBJ+JxZmkLyTcXDA4YZ9RcIl13Nw1um8xlU223j1M
ea5cMrltGtv1r+q8NTlQu53GjxJ2KERDhPWk9kPi3FHv9fon3CY/6cM/mp3tiGfpS5S5RXFVPr7F
tL88ArdCDW5gsMdswwIZDrS3q60CAwEAAaNqMGgwDgYDVR0PAQH/BAQDAgWgMB0GA1UdJQQWMBQG
CCsGAQUFBwMCBggrBgEFBQcDATAYBgNVHREEETAPgg1iLmV4YW1wbGUuY29tMB0GA1UdDgQWBBQn
drJnGQUHZJfUmy20Ak9xTwqvOjANBgkqhkiG9w0BAQsFAAOCAQEAjuX8nNR5SSiIXg6+rMgSF1dS
f0goRcD8eKoK0MxYEKUTMoipXuCn7JYNkOsCGWErsPO3+/hLX6SsawDvJFN7sNabYTGWUdQQlnHN
xCX7cVNKCc2ulNfAmqPCsv0Y/tDXLrwUORErb+n0MfhXu6c2qMmeRzvSKg4Kft9K7fEBLGII97by
IOW5OPFV9awLk76Om9oZgKN4n1hR3VrculbPRq0APMUIl13PGoYl0Zx9VoNsR95VvpYkjHyugc1I
rBMSyIGWm2p/cTr+skdkco4cBBm8mMQEK+BC+F/AkSLDDbMMdncPHqetUvaUj4p0U5RbrULovQ1K
GbBaVwUFXogaDw==
-----END CERTIFICATE-----
`;

function writeLive(name: string, fullchain: string) {
  const live = path.join(tmpRoot, 'live', name);
  fs.mkdirSync(live, { recursive: true });
  fs.writeFileSync(path.join(live, 'fullchain.pem'), fullchain);
  fs.writeFileSync(
    path.join(live, 'privkey.pem'),
    '-----BEGIN PRIVATE KEY-----\nPLACEHOLDER\n-----END PRIVATE KEY-----\n'
  );
}

before(() => {
  assert.match(new X509Certificate(COVERING_FULLCHAIN).subjectAltName || '', /a\.example\.com/);
  assert.match(new X509Certificate(COVERING_FULLCHAIN).subjectAltName || '', /b\.example\.com/);
  assert.doesNotMatch(new X509Certificate(ORPHAN_B_FULLCHAIN).subjectAltName || '', /a\.example\.com/);
  assert.match(new X509Certificate(ORPHAN_B_FULLCHAIN).subjectAltName || '', /b\.example\.com/);

  tmpRoot = fs.mkdtempSync(path.join(os.tmpdir(), 'docklift-certs-'));
  config.letsencryptPath = tmpRoot;
  writeLive('a.example.com', COVERING_FULLCHAIN);
  writeLive('b.example.com', ORPHAN_B_FULLCHAIN);
});

after(() => {
  config.letsencryptPath = prevLetsencrypt;
  if (tmpRoot && fs.existsSync(tmpRoot)) {
    fs.rmSync(tmpRoot, { recursive: true, force: true });
  }
});

test('findCoveringCertName prefers multi-SAN cert after reorder despite orphan primary folder', async () => {
  const { findCoveringCertName } = await import('./certs.js');
  assert.equal(
    findCoveringCertName(['b.example.com', 'a.example.com']),
    'a.example.com',
    'orphan live/b must not win when it does not cover a'
  );
  assert.equal(findCoveringCertName(['a.example.com', 'b.example.com']), 'a.example.com');
});

test('resolveCertName / single-host covering prefers healthier multi-SAN over orphan', async () => {
  const { resolveCertName, findCoveringCertName } = await import('./certs.js');
  // Both cover b; covering cert has more SANs → higher score
  assert.equal(resolveCertName('b.example.com'), 'a.example.com');
  assert.equal(findCoveringCertName(['b.example.com']), 'a.example.com');
  assert.equal(resolveCertName('a.example.com'), 'a.example.com');
});

test('selectCertLineage expands the covering lineage, not the orphan', async () => {
  const { selectCertLineage } = await import('./certs.js');
  const lineage = selectCertLineage(['b.example.com', 'a.example.com', 'c.example.com']);
  assert.equal(lineage.certName, 'a.example.com');
  assert.equal(lineage.covering, null, 'c is not on the cert yet');
  assert.equal(lineage.needsExpand, true);
});

test('selectCertLineage reuses covering cert without expand when SANs already match', async () => {
  const { selectCertLineage } = await import('./certs.js');
  const lineage = selectCertLineage(['b.example.com', 'a.example.com']);
  assert.equal(lineage.certName, 'a.example.com');
  assert.equal(lineage.covering, 'a.example.com');
  assert.equal(lineage.needsExpand, false);
});

test('listCertDnsNames only returns DNS SANs', async () => {
  const { listCertDnsNames } = await import('./certs.js');
  const names = listCertDnsNames('a.example.com').sort();
  assert.deepEqual(names, ['a.example.com', 'b.example.com']);
});

test('findBestExpandableCertName ties break toward richer SAN set within host set', async () => {
  const { findBestExpandableCertName } = await import('./certs.js');
  // Both a and b folders are in the host set; prefer richer covering lineage named a
  assert.equal(
    findBestExpandableCertName(['b.example.com', 'a.example.com']),
    'a.example.com'
  );
});

test('findBestExpandableCertName does not expand a foreign lineage after domain move', async () => {
  const { findBestExpandableCertName, selectCertLineage } = await import('./certs.js');
  // Service B now owns b+c; live/a still has SAN b from the old service — must not be chosen
  assert.equal(findBestExpandableCertName(['b.example.com', 'c.example.com']), 'b.example.com');
  const lineage = selectCertLineage(['b.example.com', 'c.example.com']);
  assert.equal(lineage.certName, 'b.example.com');
  assert.equal(lineage.needsExpand, true, 'orphan b covers b only — expand to add c');
});

// Newer longer-lived staging-only cert vs shorter prod-only — equal overlap=1
const PROD_ONLY = `-----BEGIN CERTIFICATE-----
MIIDLTCCAhWgAwIBAgIQJJZtOGrkr4hL26iFyAb4lzANBgkqhkiG9w0BAQsFADAbMRkwFwYDVQQD
DBBwcm9kLmV4YW1wbGUuY29tMB4XDTI2MDgxMDIxMTgyNVoXDTMxMDgxMDIxMjgyNVowGzEZMBcG
A1UEAwwQcHJvZC5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBALZL
XlMzkLTCQPTJdJdaTAnBGQq+O+dUZMJzWqIoXM1PbkTxCJZ50PdmwScDI4kO63Biz0wsLZ0155tb
yADWQY81AhZciZMxVBR10Jnwm4hYR07GqgwFDaYm7guCP8JRYA8pPXK1fy9RYNMlpNl9AG3ONr/L
WcI8qwrY4/nq7c+OHUUYwDSAPwEMo4hSvKLLrr8r8J+0o8pQnBhIVydjsC5H3NyZ5tquCPhSBCOD
4LvCdr9JdOI8Wpv8KQlino3PHl5VgLLFpWmZVnxPUxQOmqiwCmCBJg0z8IYCRQB6Rdh1kiZJmGnK
vT+X+S/IyNMAn/rL96v6L5leoNjJeHswju0CAwEAAaNtMGswDgYDVR0PAQH/BAQDAgWgMB0GA1Ud
JQQWMBQGCCsGAQUFBwMCBggrBgEFBQcDATAbBgNVHREEFDASghBwcm9kLmV4YW1wbGUuY29tMB0G
A1UdDgQWBBSrxajfRxJ4xCCbOaebGYTdn3U7LjANBgkqhkiG9w0BAQsFAAOCAQEAYDnBAf3JCroT
yZumhGel/+EmLbG+J2/BXflx2v5rKvGBxXXUgtDRi/pXjymyVFEc9EYU8YRdpO3i7mwNj7NdYQuu
N1Y0TmSbE5zyKGOoyPzSLpPbpwG9njRKKjBdd2h5VNroydqFtCzjoBQwDdpvlzkjcnlbux6lTcn5
a0zzhRk9McjnGLi0IFd+OgikAGGCi/z4YSZ78ojWxTI6/9AYTlEa9isnaaer0pP6SAoSwrCAQG+h
aucj5kB69n0WRAZ2ja7bCS5BjeWvgp8pnoK4ks3dv9i6cJe6kJEFaasu1ZSQJ48xiDTe66e80HDP
5I5lRHoAa5lPoiTuMm9U7DxKeA==
-----END CERTIFICATE-----
`;

const STAGING_ONLY = `-----BEGIN CERTIFICATE-----
MIIDNjCCAh6gAwIBAgIQWmagb8crFrJPpwY7G5EKJTANBgkqhkiG9w0BAQsFADAeMRwwGgYDVQQD
DBNzdGFnaW5nLmV4YW1wbGUuY29tMB4XDTI2MDgxMDIxMTgyNVoXDTM2MDgxMDIxMjgyNlowHjEc
MBoGA1UEAwwTc3RhZ2luZy5leGFtcGxlLmNvbTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoC
ggEBAMEnPgMPD7egfRJ2CIAwOyaREGFeOaOR4IHy4p9cneuZE19MHM8Cl579velKJqF5sCpesHoY
Erpimfq90DeyWBGZ8qPa0gH+hFmwg2KYV1F+gvu6hErgRuDcptoAkW/qubUpruc0xkV72cFXvoxp
j4bE5BMdFDg/f1YJB/WRK7bwSgmCtuN54EGolPptEdxjQApUnLYrTjSvRCM4Gw+wUdCVLPxbQcKY
WQzXV8oh5ApDhKDR8NJJvgiSaec5+ynw1aOfFiB/l3T1A8W26XedHQzQHnQAkEOVEQR9sOK1ZuYh
+8WPur9oGNBT8+4In3Bs3XyM6K9vWDwOixw0/r4jvVkCAwEAAaNwMG4wDgYDVR0PAQH/BAQDAgWg
MB0GA1UdJQQWMBQGCCsGAQUFBwMCBggrBgEFBQcDATAeBgNVHREEFzAVghNzdGFnaW5nLmV4YW1w
bGUuY29tMB0GA1UdDgQWBBSyUpP1dA8UuXrCWVYE/r0su8r5FjANBgkqhkiG9w0BAQsFAAOCAQEA
hlX5Llex9HQWWRrP457HafnGVh5Rfrn88prhNj/hIDU4oFIGF53MMTwUcJMvxhyfij5knChU0gJJ
Oi16itHdLw4cX2NXHK4hWmEKcawqB5ULUNFmXWg2k7Li9FS6J1VSH1t9m1r4DA5GKPhBPvKCwllA
T4YJ0JtTkby8bR6YWCRdXd9M6i9DTFF0rCUs7es4yNcfdofbffI0yjQiPMut9CPPf6+XCJZF+/xA
EIVxxv03X9UdGI+tVNly6UOJvCXGvchWLk8q6OUAoh7xyEd5qxIye+KWVUd3/fVH8aP60rp4GOWB
PQMYts9rbiPlFDK32Gv0wD1RwzgcpA6MRt5pPA==
-----END CERTIFICATE-----
`;

test('equal-overlap expand prefers primary-covering lineage over newer orphan', async () => {
  writeLive('prod.example.com', PROD_ONLY);
  writeLive('staging.example.com', STAGING_ONLY);
  const { findBestExpandableCertName, selectCertLineage } = await import('./certs.js');
  assert.equal(
    findBestExpandableCertName(['prod.example.com', 'staging.example.com']),
    'prod.example.com',
    'must not pick newer staging-only cert when primary is prod'
  );
  assert.equal(
    selectCertLineage(['prod.example.com', 'staging.example.com']).certName,
    'prod.example.com'
  );
});
